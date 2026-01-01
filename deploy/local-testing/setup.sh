#!/bin/bash
# CertWatch Local Testing Setup Script
# This script sets up a local Kind cluster with cert-manager, Prometheus, and sample apps
#
# Prerequisites:
# - Docker running
# - kind installed
# - kubectl installed
# - helm installed

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

# Parse arguments
SKIP_CLUSTER=false
SKIP_CERTMANAGER=false
SKIP_PROMETHEUS=false
SKIP_SAMPLEAPPS=false
SKIP_AGENT=false
CLEANUP=false

while [[ $# -gt 0 ]]; do
    case $1 in
        --skip-cluster) SKIP_CLUSTER=true ;;
        --skip-certmanager) SKIP_CERTMANAGER=true ;;
        --skip-prometheus) SKIP_PROMETHEUS=true ;;
        --skip-sample-apps) SKIP_SAMPLEAPPS=true ;;
        --skip-agent) SKIP_AGENT=true ;;
        --cleanup) CLEANUP=true ;;
        *) echo "Unknown option: $1"; exit 1 ;;
    esac
    shift
done

echo -e "${CYAN}================================================${NC}"
echo -e "${CYAN}  CertWatch Local Testing Setup${NC}"
echo -e "${CYAN}================================================${NC}"
echo ""

# Cleanup mode
if [ "$CLEANUP" = true ]; then
    echo -e "${YELLOW}Cleaning up...${NC}"
    kind delete cluster --name certwatch-test 2>/dev/null || true
    echo -e "${GREEN}Cluster deleted.${NC}"
    exit 0
fi

# Check prerequisites
check_command() {
    if ! command -v $1 &> /dev/null; then
        echo -e "${RED}ERROR: $1 is not installed${NC}"
        return 1
    fi
    return 0
}

echo -e "${YELLOW}Checking prerequisites...${NC}"

check_command docker || exit 1
check_command kind || { echo -e "${YELLOW}Install with: brew install kind${NC}"; exit 1; }
check_command kubectl || { echo -e "${YELLOW}Install with: brew install kubectl${NC}"; exit 1; }
check_command helm || { echo -e "${YELLOW}Install with: brew install helm${NC}"; exit 1; }

# Check if Docker is running
if ! docker info &> /dev/null; then
    echo -e "${RED}ERROR: Docker is not running. Please start Docker.${NC}"
    exit 1
fi

echo -e "${GREEN}All prerequisites met!${NC}"
echo ""

# Step 1: Create Kind cluster
if [ "$SKIP_CLUSTER" = false ]; then
    echo -e "${CYAN}================================================${NC}"
    echo -e "${CYAN}Step 1: Creating Kind cluster...${NC}"
    echo -e "${CYAN}================================================${NC}"

    # Check if cluster already exists
    if kind get clusters 2>/dev/null | grep -q "certwatch-test"; then
        echo -e "${YELLOW}Cluster 'certwatch-test' already exists. Deleting...${NC}"
        kind delete cluster --name certwatch-test
    fi

    kind create cluster --config "$SCRIPT_DIR/kind-config.yaml"

    echo -e "${YELLOW}Waiting for cluster to be ready...${NC}"
    kubectl wait --for=condition=Ready nodes --all --timeout=120s

    echo -e "${GREEN}Kind cluster created successfully!${NC}"
    echo ""
fi

# Step 2: Install cert-manager
if [ "$SKIP_CERTMANAGER" = false ]; then
    echo -e "${CYAN}================================================${NC}"
    echo -e "${CYAN}Step 2: Installing cert-manager...${NC}"
    echo -e "${CYAN}================================================${NC}"

    kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.16.0/cert-manager.yaml

    echo -e "${YELLOW}Waiting for cert-manager to be ready...${NC}"
    kubectl wait --for=condition=Available deployment/cert-manager -n cert-manager --timeout=120s
    kubectl wait --for=condition=Available deployment/cert-manager-webhook -n cert-manager --timeout=120s
    kubectl wait --for=condition=Available deployment/cert-manager-cainjector -n cert-manager --timeout=120s

    # Wait for webhook to be fully ready
    sleep 10

    echo -e "${YELLOW}Creating issuers...${NC}"
    kubectl apply -f "$SCRIPT_DIR/cert-manager-issuers.yaml"

    # Wait for local CA to be ready
    echo -e "${YELLOW}Waiting for local CA to be ready...${NC}"
    sleep 15
    kubectl wait --for=condition=Ready certificate/local-ca -n cert-manager --timeout=60s 2>/dev/null || true

    echo -e "${GREEN}cert-manager installed successfully!${NC}"
    echo ""
fi

# Step 3: Install Prometheus
if [ "$SKIP_PROMETHEUS" = false ]; then
    echo -e "${CYAN}================================================${NC}"
    echo -e "${CYAN}Step 3: Installing Prometheus stack...${NC}"
    echo -e "${CYAN}================================================${NC}"

    # Add Helm repo
    helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
    helm repo update

    # Create monitoring namespace
    kubectl create namespace monitoring --dry-run=client -o yaml | kubectl apply -f -

    # Install kube-prometheus-stack
    helm upgrade --install prometheus prometheus-community/kube-prometheus-stack \
        --namespace monitoring \
        --values "$SCRIPT_DIR/prometheus-values.yaml" \
        --wait --timeout 5m || echo -e "${YELLOW}WARNING: Prometheus installation may have issues. Continuing...${NC}"

    echo -e "${GREEN}Prometheus stack installed successfully!${NC}"
    echo ""
fi

# Step 4: Deploy sample apps
if [ "$SKIP_SAMPLEAPPS" = false ]; then
    echo -e "${CYAN}================================================${NC}"
    echo -e "${CYAN}Step 4: Deploying sample applications...${NC}"
    echo -e "${CYAN}================================================${NC}"

    kubectl apply -f "$SCRIPT_DIR/sample-apps.yaml"

    echo -e "${YELLOW}Waiting for sample apps to be ready...${NC}"
    kubectl wait --for=condition=Available deployment/nginx-demo -n demo-apps --timeout=120s
    kubectl wait --for=condition=Available deployment/httpbin -n demo-apps --timeout=120s
    kubectl wait --for=condition=Available deployment/echo-server -n demo-apps --timeout=120s

    echo -e "${YELLOW}Creating sample certificates...${NC}"
    kubectl apply -f "$SCRIPT_DIR/certificates.yaml"

    # Wait for certificates
    sleep 10
    echo -e "${YELLOW}Waiting for certificates to be issued...${NC}"
    kubectl wait --for=condition=Ready certificate/nginx-demo-tls -n demo-apps --timeout=120s 2>/dev/null || true
    kubectl wait --for=condition=Ready certificate/httpbin-tls -n demo-apps --timeout=120s 2>/dev/null || true
    kubectl wait --for=condition=Ready certificate/echo-server-tls -n demo-apps --timeout=120s 2>/dev/null || true

    echo -e "${GREEN}Sample apps deployed successfully!${NC}"
    echo ""
fi

# Step 5: Build and deploy cw-agent-certmanager
if [ "$SKIP_AGENT" = false ]; then
    echo -e "${CYAN}================================================${NC}"
    echo -e "${CYAN}Step 5: Building and deploying cw-agent-certmanager...${NC}"
    echo -e "${CYAN}================================================${NC}"

    # Build the agent
    pushd "$SCRIPT_DIR/../.." > /dev/null
    echo -e "${YELLOW}Building cw-agent-certmanager...${NC}"

    # Build for Linux (for container)
    GOOS=linux GOARCH=amd64 go build -o bin/cw-agent-certmanager-linux-amd64 ./cmd/cw-agent-certmanager

    # Create simple Dockerfile for local testing
    cat > Dockerfile.local << 'EOF'
FROM gcr.io/distroless/base-debian12:nonroot
COPY bin/cw-agent-certmanager-linux-amd64 /cw-agent-certmanager
USER nonroot:nonroot
ENTRYPOINT ["/cw-agent-certmanager"]
EOF

    # Build Docker image
    echo -e "${YELLOW}Building Docker image...${NC}"
    docker build -f Dockerfile.local -t cw-agent-certmanager:dev .

    # Load image into Kind
    echo -e "${YELLOW}Loading image into Kind cluster...${NC}"
    kind load docker-image cw-agent-certmanager:dev --name certwatch-test

    popd > /dev/null

    # Deploy agent
    echo -e "${YELLOW}Deploying cw-agent-certmanager...${NC}"
    kubectl apply -f "$SCRIPT_DIR/cw-agent-certmanager.yaml"

    echo -e "${YELLOW}Waiting for agent to be ready...${NC}"
    kubectl wait --for=condition=Available deployment/cw-agent-certmanager -n certwatch --timeout=120s

    echo -e "${GREEN}cw-agent-certmanager deployed successfully!${NC}"
    echo ""
fi

# Summary
echo -e "${GREEN}================================================${NC}"
echo -e "${GREEN}  Setup Complete!${NC}"
echo -e "${GREEN}================================================${NC}"
echo ""
echo -e "${CYAN}Access URLs:${NC}"
echo "  - Prometheus:    http://localhost:30090"
echo "  - Grafana:       http://localhost:30030  (admin/certwatch123)"
echo "  - Agent Metrics: http://localhost:30402/metrics"
echo ""
echo -e "${CYAN}Useful commands:${NC}"
echo "  # View certificates"
echo "  kubectl get certificates -A"
echo ""
echo "  # View agent logs"
echo "  kubectl logs -f deployment/cw-agent-certmanager -n certwatch"
echo ""
echo "  # View agent metrics"
echo "  curl http://localhost:30402/metrics"
echo ""
echo "  # Port-forward Prometheus (alternative)"
echo "  kubectl port-forward svc/prometheus-kube-prometheus-prometheus -n monitoring 9090:9090"
echo ""
echo "  # Cleanup"
echo "  ./setup.sh --cleanup"
echo ""

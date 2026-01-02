# CertWatch Local Testing Setup Script for Windows
# This script sets up a local Kind cluster with cert-manager, Prometheus, and sample apps
#
# Prerequisites:
# - Docker Desktop running
# - kind installed (winget install Kubernetes.kind)
# - kubectl installed (winget install Kubernetes.kubectl)
# - helm installed (winget install Helm.Helm)

param(
    [switch]$SkipCluster,
    [switch]$SkipCertManager,
    [switch]$SkipPrometheus,
    [switch]$SkipSampleApps,
    [switch]$SkipAgent,
    [switch]$Cleanup
)

$ErrorActionPreference = "Stop"
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path

Write-Host "================================================" -ForegroundColor Cyan
Write-Host "  CertWatch Local Testing Setup" -ForegroundColor Cyan
Write-Host "================================================" -ForegroundColor Cyan
Write-Host ""

# Cleanup mode
if ($Cleanup) {
    Write-Host "Cleaning up..." -ForegroundColor Yellow
    kind delete cluster --name certwatch-test 2>$null
    Write-Host "Cluster deleted." -ForegroundColor Green
    exit 0
}

# Check prerequisites
function Test-Command {
    param([string]$Command)
    $null = Get-Command $Command -ErrorAction SilentlyContinue
    return $?
}

Write-Host "Checking prerequisites..." -ForegroundColor Yellow

if (-not (Test-Command "docker")) {
    Write-Host "ERROR: Docker is not installed or not in PATH" -ForegroundColor Red
    Write-Host "Install Docker Desktop from https://www.docker.com/products/docker-desktop/" -ForegroundColor Yellow
    exit 1
}

# Check if Docker is running
docker version --format "{{.Server.Version}}" 2>$null | Out-Null
if ($LASTEXITCODE -ne 0) {
    Write-Host "ERROR: Docker is not running. Please start Docker Desktop." -ForegroundColor Red
    exit 1
}

if (-not (Test-Command "kind")) {
    Write-Host "ERROR: kind is not installed" -ForegroundColor Red
    Write-Host "Install with: winget install Kubernetes.kind" -ForegroundColor Yellow
    exit 1
}

if (-not (Test-Command "kubectl")) {
    Write-Host "ERROR: kubectl is not installed" -ForegroundColor Red
    Write-Host "Install with: winget install Kubernetes.kubectl" -ForegroundColor Yellow
    exit 1
}

if (-not (Test-Command "helm")) {
    Write-Host "ERROR: helm is not installed" -ForegroundColor Red
    Write-Host "Install with: winget install Helm.Helm" -ForegroundColor Yellow
    exit 1
}

Write-Host "All prerequisites met!" -ForegroundColor Green
Write-Host ""

# Step 1: Create Kind cluster
if (-not $SkipCluster) {
    Write-Host "================================================" -ForegroundColor Cyan
    Write-Host "Step 1: Creating Kind cluster..." -ForegroundColor Cyan
    Write-Host "================================================" -ForegroundColor Cyan

    # Check if cluster already exists
    $ErrorActionPreference = "Continue"
    $clusters = (kind get clusters 2>&1) -join "`n"
    $ErrorActionPreference = "Stop"
    if ($clusters -match "certwatch-test") {
        Write-Host "Cluster 'certwatch-test' already exists. Deleting..." -ForegroundColor Yellow
        kind delete cluster --name certwatch-test
    }

    kind create cluster --config "$ScriptDir\kind-config.yaml"
    if ($LASTEXITCODE -ne 0) {
        Write-Host "ERROR: Failed to create Kind cluster" -ForegroundColor Red
        exit 1
    }

    Write-Host "Waiting for cluster to be ready..." -ForegroundColor Yellow
    kubectl wait --for=condition=Ready nodes --all --timeout=120s

    Write-Host "Kind cluster created successfully!" -ForegroundColor Green
    Write-Host ""
}

# Step 2: Install cert-manager
if (-not $SkipCertManager) {
    Write-Host "================================================" -ForegroundColor Cyan
    Write-Host "Step 2: Installing cert-manager..." -ForegroundColor Cyan
    Write-Host "================================================" -ForegroundColor Cyan

    kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.16.0/cert-manager.yaml

    Write-Host "Waiting for cert-manager to be ready..." -ForegroundColor Yellow
    kubectl wait --for=condition=Available deployment/cert-manager -n cert-manager --timeout=120s
    kubectl wait --for=condition=Available deployment/cert-manager-webhook -n cert-manager --timeout=120s
    kubectl wait --for=condition=Available deployment/cert-manager-cainjector -n cert-manager --timeout=120s

    # Wait a bit for webhook to be fully ready
    Start-Sleep -Seconds 10

    Write-Host "Creating issuers..." -ForegroundColor Yellow
    kubectl apply -f "$ScriptDir\cert-manager-issuers.yaml"

    # Wait for local CA to be ready
    Write-Host "Waiting for local CA to be ready..." -ForegroundColor Yellow
    Start-Sleep -Seconds 15
    kubectl wait --for=condition=Ready certificate/local-ca -n cert-manager --timeout=60s 2>$null

    Write-Host "cert-manager installed successfully!" -ForegroundColor Green
    Write-Host ""
}

# Step 3: Install Prometheus
if (-not $SkipPrometheus) {
    Write-Host "================================================" -ForegroundColor Cyan
    Write-Host "Step 3: Installing Prometheus stack..." -ForegroundColor Cyan
    Write-Host "================================================" -ForegroundColor Cyan

    # Add Helm repo
    helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
    helm repo update

    # Create monitoring namespace
    kubectl create namespace monitoring --dry-run=client -o yaml | kubectl apply -f -

    # Install kube-prometheus-stack
    helm upgrade --install prometheus prometheus-community/kube-prometheus-stack `
        --namespace monitoring `
        --values "$ScriptDir\prometheus-values.yaml" `
        --wait --timeout 5m

    if ($LASTEXITCODE -ne 0) {
        Write-Host "WARNING: Prometheus installation may have issues. Continuing..." -ForegroundColor Yellow
    } else {
        Write-Host "Prometheus stack installed successfully!" -ForegroundColor Green
    }
    Write-Host ""
}

# Step 4: Deploy sample apps
if (-not $SkipSampleApps) {
    Write-Host "================================================" -ForegroundColor Cyan
    Write-Host "Step 4: Deploying sample applications..." -ForegroundColor Cyan
    Write-Host "================================================" -ForegroundColor Cyan

    kubectl apply -f "$ScriptDir\sample-apps.yaml"

    Write-Host "Waiting for sample apps to be ready..." -ForegroundColor Yellow
    kubectl wait --for=condition=Available deployment/nginx-demo -n demo-apps --timeout=120s
    kubectl wait --for=condition=Available deployment/httpbin -n demo-apps --timeout=120s
    kubectl wait --for=condition=Available deployment/echo-server -n demo-apps --timeout=120s

    Write-Host "Creating sample certificates..." -ForegroundColor Yellow
    kubectl apply -f "$ScriptDir\certificates.yaml"

    # Wait for certificates
    Start-Sleep -Seconds 10
    Write-Host "Waiting for certificates to be issued..." -ForegroundColor Yellow
    kubectl wait --for=condition=Ready certificate/nginx-demo-tls -n demo-apps --timeout=120s 2>$null
    kubectl wait --for=condition=Ready certificate/httpbin-tls -n demo-apps --timeout=120s 2>$null
    kubectl wait --for=condition=Ready certificate/echo-server-tls -n demo-apps --timeout=120s 2>$null

    Write-Host "Sample apps deployed successfully!" -ForegroundColor Green
    Write-Host ""
}

# Step 5: Build and deploy cw-agent-certmanager
if (-not $SkipAgent) {
    Write-Host "================================================" -ForegroundColor Cyan
    Write-Host "Step 5: Building and deploying cw-agent-certmanager..." -ForegroundColor Cyan
    Write-Host "================================================" -ForegroundColor Cyan

    # Build the agent
    Push-Location "$ScriptDir\..\.."
    Write-Host "Building cw-agent-certmanager..." -ForegroundColor Yellow

    # Build for Linux (for container)
    $env:GOOS = "linux"
    $env:GOARCH = "amd64"
    go build -o bin/cw-agent-certmanager-linux-amd64 ./cmd/cw-agent-certmanager
    if ($LASTEXITCODE -ne 0) {
        Write-Host "ERROR: Failed to build cw-agent-certmanager" -ForegroundColor Red
        Pop-Location
        exit 1
    }

    # Reset env
    $env:GOOS = ""
    $env:GOARCH = ""

    # Create simple Dockerfile for local testing
    $dockerfile = @"
FROM gcr.io/distroless/base-debian12:nonroot
COPY bin/cw-agent-certmanager-linux-amd64 /cw-agent-certmanager
USER nonroot:nonroot
ENTRYPOINT ["/cw-agent-certmanager"]
"@
    $dockerfile | Out-File -FilePath "Dockerfile.local" -Encoding utf8 -NoNewline

    # Build Docker image
    Write-Host "Building Docker image..." -ForegroundColor Yellow
    docker build -f Dockerfile.local -t cw-agent-certmanager:dev .
    if ($LASTEXITCODE -ne 0) {
        Write-Host "ERROR: Failed to build Docker image" -ForegroundColor Red
        Pop-Location
        exit 1
    }

    # Load image into Kind
    Write-Host "Loading image into Kind cluster..." -ForegroundColor Yellow
    kind load docker-image cw-agent-certmanager:dev --name certwatch-test
    if ($LASTEXITCODE -ne 0) {
        Write-Host "ERROR: Failed to load image into Kind" -ForegroundColor Red
        Pop-Location
        exit 1
    }

    Pop-Location

    # Deploy agent
    Write-Host "Deploying cw-agent-certmanager..." -ForegroundColor Yellow
    kubectl apply -f "$ScriptDir\cw-agent-certmanager.yaml"

    Write-Host "Waiting for agent to be ready..." -ForegroundColor Yellow
    kubectl wait --for=condition=Available deployment/cw-agent-certmanager -n certwatch --timeout=120s

    Write-Host "cw-agent-certmanager deployed successfully!" -ForegroundColor Green
    Write-Host ""
}

# Summary
Write-Host "================================================" -ForegroundColor Green
Write-Host "  Setup Complete!" -ForegroundColor Green
Write-Host "================================================" -ForegroundColor Green
Write-Host ""
Write-Host "Access URLs:" -ForegroundColor Cyan
Write-Host "  - Prometheus:  http://localhost:30090" -ForegroundColor White
Write-Host "  - Grafana:     http://localhost:30030  (admin/certwatch123)" -ForegroundColor White
Write-Host "  - Agent Metrics: http://localhost:30402/metrics" -ForegroundColor White
Write-Host ""
Write-Host "Useful commands:" -ForegroundColor Cyan
Write-Host "  # View certificates" -ForegroundColor Gray
Write-Host "  kubectl get certificates -A" -ForegroundColor White
Write-Host ""
Write-Host "  # View agent logs" -ForegroundColor Gray
Write-Host "  kubectl logs -f deployment/cw-agent-certmanager -n certwatch" -ForegroundColor White
Write-Host ""
Write-Host "  # View agent metrics" -ForegroundColor Gray
Write-Host "  curl http://localhost:30402/metrics" -ForegroundColor White
Write-Host ""
Write-Host "  # Port-forward Prometheus (alternative)" -ForegroundColor Gray
Write-Host "  kubectl port-forward svc/prometheus-kube-prometheus-prometheus -n monitoring 9090:9090" -ForegroundColor White
Write-Host ""
Write-Host "  # Cleanup" -ForegroundColor Gray
Write-Host "  .\setup.ps1 -Cleanup" -ForegroundColor White
Write-Host ""

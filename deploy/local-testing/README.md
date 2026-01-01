# CertWatch Local Testing Environment

This directory contains everything needed to test the `cw-agent-certmanager` controller locally using Kind (Kubernetes in Docker).

## Prerequisites

### Windows

```powershell
# Install Docker Desktop
# Download from: https://www.docker.com/products/docker-desktop/

# Install kind, kubectl, helm using winget
winget install Kubernetes.kind
winget install Kubernetes.kubectl
winget install Helm.Helm
```

### macOS

```bash
brew install kind kubectl helm
# Docker Desktop: https://www.docker.com/products/docker-desktop/
```

### Linux

```bash
# kind
curl -Lo ./kind https://kind.sigs.k8s.io/dl/v0.24.0/kind-linux-amd64
chmod +x ./kind
sudo mv ./kind /usr/local/bin/kind

# kubectl
curl -LO "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl"
chmod +x kubectl
sudo mv kubectl /usr/local/bin/

# helm
curl https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash
```

## Quick Start

### Windows (PowerShell)

```powershell
cd cw-agent\deploy\local-testing
.\setup.ps1
```

### macOS/Linux

```bash
cd cw-agent/deploy/local-testing
chmod +x setup.sh
./setup.sh
```

## What Gets Installed

The setup script creates a complete testing environment:

1. **Kind Cluster** (`certwatch-test`)
   - Single-node cluster with port mappings for ingress and NodePort services

2. **cert-manager** (v1.16.0)
   - Self-signed ClusterIssuer
   - Local CA ClusterIssuer (for testing certificate chains)
   - Let's Encrypt Staging ClusterIssuer (for internet-exposed testing)

3. **Sample Applications** (namespace: `demo-apps`)
   - nginx-demo - Simple web server
   - httpbin - API testing
   - echo-server - Echo service

4. **Sample Certificates**
   - nginx-demo-tls (RSA 2048, 90-day)
   - httpbin-tls (ECDSA 256, 30-day)
   - echo-server-tls (RSA 4096, 180-day, multi-domain)
   - short-lived-cert (24-hour, for testing renewal)
   - selfsigned-direct (self-signed, 1-year)

5. **Prometheus Stack** (namespace: `monitoring`)
   - Prometheus (port 30090)
   - Grafana (port 30030)
   - AlertManager (port 30903)
   - Pre-configured CertWatch dashboard

6. **cw-agent-certmanager** (namespace: `certwatch`)
   - Controller watching all certificates
   - Metrics exposed on port 30402
   - ServiceMonitor for Prometheus scraping

## Access URLs

| Service | URL | Credentials |
|---------|-----|-------------|
| Prometheus | http://localhost:30090 | - |
| Grafana | http://localhost:30030 | admin / certwatch123 |
| Agent Metrics | http://localhost:30402/metrics | - |

## Useful Commands

### View Certificates

```bash
# List all certificates
kubectl get certificates -A

# Describe a specific certificate
kubectl describe certificate nginx-demo-tls -n demo-apps

# Check certificate ready status
kubectl get certificates -A -o wide
```

### View Agent

```bash
# Agent logs
kubectl logs -f deployment/cw-agent-certmanager -n certwatch

# Agent pod status
kubectl get pods -n certwatch

# Agent metrics
curl http://localhost:30402/metrics

# Filter certwatch metrics
curl -s http://localhost:30402/metrics | grep certwatch
```

### Prometheus Queries

Access Prometheus at http://localhost:30090 and try these queries:

```promql
# Certificates being watched
certwatch_certmanager_certificates_watched

# Certificate ready status (1=ready, 0=not ready)
certwatch_certmanager_certificate_ready

# Days until expiry
certwatch_certmanager_certificate_days_until_expiry

# Reconcile rate
rate(certwatch_certmanager_reconcile_total[5m])

# Sync operations
rate(certwatch_certmanager_sync_total[5m])

# Failed issuance attempts
certwatch_certmanager_certificate_failed_attempts
```

### Grafana Dashboard

1. Open http://localhost:30030
2. Login with admin / certwatch123
3. Navigate to Dashboards → CertWatch → CertWatch Agent - cert-manager

## Testing Scenarios

### Test Certificate Renewal

The `short-lived-cert` certificate has a 24-hour duration with 8-hour renewal window. You can watch it renew:

```bash
# Watch certificate status
watch kubectl get certificate short-lived-cert -n demo-apps

# Check renewal logs
kubectl logs -f deployment/cw-agent-certmanager -n certwatch | grep short-lived
```

### Test Certificate Failure

Create a certificate with an invalid issuer:

```yaml
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: failing-cert
  namespace: demo-apps
spec:
  secretName: failing-tls
  issuerRef:
    name: nonexistent-issuer
    kind: ClusterIssuer
  commonName: fail.local
  dnsNames:
    - fail.local
```

```bash
kubectl apply -f - <<EOF
# paste yaml above
EOF

# Watch failure attempts
kubectl get certificate failing-cert -n demo-apps -w
```

### Test New Certificate

```bash
# Create a new certificate
kubectl apply -f - <<EOF
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: my-new-cert
  namespace: demo-apps
spec:
  secretName: my-new-tls
  issuerRef:
    name: local-ca-issuer
    kind: ClusterIssuer
  commonName: mynew.local
  dnsNames:
    - mynew.local
EOF

# Watch agent pick it up
kubectl logs -f deployment/cw-agent-certmanager -n certwatch

# Verify in metrics
curl -s http://localhost:30402/metrics | grep my-new
```

## Configuration

### Agent Configuration

The agent configuration is in the ConfigMap:

```bash
kubectl get configmap cw-agent-certmanager-config -n certwatch -o yaml
```

To modify:

```bash
kubectl edit configmap cw-agent-certmanager-config -n certwatch
kubectl rollout restart deployment/cw-agent-certmanager -n certwatch
```

### API Key

Set your real API key:

```bash
kubectl create secret generic cw-agent-certmanager-secret \
  --namespace certwatch \
  --from-literal=api-key=your_actual_api_key \
  --dry-run=client -o yaml | kubectl apply -f -

kubectl rollout restart deployment/cw-agent-certmanager -n certwatch
```

## Cleanup

### Windows

```powershell
.\setup.ps1 -Cleanup
```

### macOS/Linux

```bash
./setup.sh --cleanup
```

### Manual Cleanup

```bash
kind delete cluster --name certwatch-test
```

## Troubleshooting

### Docker not running

```
ERROR: Docker is not running
```

Start Docker Desktop and wait for it to be ready.

### Port already in use

If ports 80, 443, or 30xxx are already in use, modify `kind-config.yaml` port mappings.

### cert-manager webhook issues

If certificate creation fails with webhook errors:

```bash
# Restart cert-manager
kubectl rollout restart deployment/cert-manager-webhook -n cert-manager

# Wait and retry
sleep 30
kubectl apply -f certificates.yaml
```

### Agent can't connect to API

The agent is configured with a test API key. For real sync testing, update the secret with your actual API key.

### Prometheus not scraping agent

Check if the ServiceMonitor is created:

```bash
kubectl get servicemonitor -n certwatch
```

Check Prometheus targets at http://localhost:30090/targets

## Files in This Directory

| File | Description |
|------|-------------|
| `kind-config.yaml` | Kind cluster configuration with port mappings |
| `sample-apps.yaml` | Sample application deployments |
| `cert-manager-issuers.yaml` | ClusterIssuers for local and Let's Encrypt |
| `certificates.yaml` | Sample certificates for testing |
| `prometheus-values.yaml` | Prometheus Helm chart values |
| `cw-agent-certmanager.yaml` | Agent deployment and RBAC |
| `setup.ps1` | Windows PowerShell setup script |
| `setup.sh` | macOS/Linux bash setup script |

## References

- [Kind Quick Start](https://kind.sigs.k8s.io/docs/user/quick-start/)
- [cert-manager Documentation](https://cert-manager.io/docs/)
- [Prometheus Helm Chart](https://github.com/prometheus-community/helm-charts)
- [kube-prometheus-stack](https://github.com/prometheus-operator/kube-prometheus)

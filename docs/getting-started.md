# Getting Started with CertWatch Agent

This guide will help you get CertWatch Agent running in minutes.

## Prerequisites

- [CertWatch account](https://certwatch.app) with an API key
- One of the following:
  - **CLI**: Linux, macOS, or Windows with Go 1.21+
  - **Docker**: Docker Engine 20.10+
  - **Kubernetes**: Kubernetes 1.19+ with Helm 3.8+

## Quick Start

### Option 1: CLI Installation

**Linux/macOS (Quick Install):**

```bash
curl -sSL https://certwatch.app/install.sh | bash
```

**Using Go:**

```bash
go install github.com/certwatch-app/cw-agent/cmd/cw-agent@latest
```

**Using Homebrew (macOS/Linux):**

```bash
brew install certwatch-app/tap/cw-agent
```

Then run the interactive setup wizard:

```bash
cw-agent init
```

This will guide you through:
- Setting your API key
- Configuring agent name
- Adding certificates to monitor

Start monitoring:

```bash
cw-agent start -c certwatch.yaml
```

### Option 2: Docker

```bash
# Create config file first (see Configuration below)
docker run -v $(pwd)/certwatch.yaml:/etc/certwatch/certwatch.yaml \
  ghcr.io/certwatch-app/cw-agent:latest
```

### Option 3: Kubernetes (Helm)

**Network Scanner Only:**

```bash
helm install cw-agent oci://ghcr.io/certwatch-app/helm-charts/cw-agent \
  --namespace certwatch --create-namespace \
  --set agent.name=my-cluster \
  --set apiKey.value=cw_your_api_key \
  --set certificates[0].hostname=api.example.com
```

**cert-manager Integration Only:**

```bash
helm install cw-agent-certmanager oci://ghcr.io/certwatch-app/helm-charts/cw-agent-certmanager \
  --namespace certwatch --create-namespace \
  --set agent.name=my-cluster \
  --set apiKey.value=cw_your_api_key
```

**Both (Recommended for Kubernetes):**

```bash
helm install certwatch oci://ghcr.io/certwatch-app/helm-charts/cw-stack \
  --namespace certwatch --create-namespace \
  --set global.apiKey.value=cw_your_api_key \
  --set agent.enabled=true \
  --set certManager.enabled=true \
  --set cw-agent.agent.name=network-scanner \
  --set cw-agent-certmanager.agent.name=k8s-cluster
```

For production deployments, see the [Kubernetes Guide](kubernetes.md).

## Configuration

### Minimal Configuration

Create `certwatch.yaml`:

```yaml
api:
  key: "cw_your_api_key_here"

agent:
  name: "my-agent"

certificates:
  - hostname: "example.com"
    port: 443
```

### Full Configuration Example

```yaml
api:
  endpoint: "https://api.certwatch.app"
  key: "cw_your_api_key_here"
  timeout: 30s

agent:
  name: "production-monitor"
  sync_interval: 5m      # How often to sync with cloud
  scan_interval: 1m      # How often to scan certificates
  concurrency: 10        # Max concurrent scans
  log_level: info        # debug, info, warn, error
  metrics_port: 8080     # Prometheus metrics (0 to disable)
  heartbeat_interval: 30s # Agent offline alerts (0 to disable)

certificates:
  - hostname: "api.example.com"
    port: 443
    tags: ["production", "api"]
    notes: "Main API endpoint"

  - hostname: "web.example.com"
    port: 443
    tags: ["production", "web"]
```

## Getting Your API Key

1. Log in to [CertWatch](https://certwatch.app)
2. Go to **Settings** > **API Keys**
3. Create a new key with the `cloud:sync` scope
4. Copy the key (it's only shown once!)

## Next Steps

- [CLI Reference](cli-reference.md) - All commands and options
- [Kubernetes Guide](kubernetes.md) - Production Helm deployments
- [cert-manager Integration](cert-manager.md) - Monitor cert-manager certificates
- [Metrics Reference](metrics.md) - Prometheus metrics and health endpoints

## Troubleshooting

### Agent not syncing

Check the logs:

```bash
# CLI
cw-agent start -c certwatch.yaml  # Look for error messages

# Kubernetes
kubectl logs -n certwatch -l app.kubernetes.io/name=cw-agent -f
```

### Connection refused

Ensure your API key is valid and the endpoint is reachable:

```bash
curl -H "Authorization: Bearer cw_your_api_key" \
  https://api.certwatch.app/v1/health
```

### Certificate not appearing in dashboard

1. Check that the hostname is reachable from where the agent runs
2. Verify the port is correct (usually 443)
3. Check agent logs for scan errors
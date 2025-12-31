# CertWatch Agent Helm Chart

SSL/TLS certificate monitoring agent for [CertWatch](https://certwatch.app).

## Prerequisites

- Kubernetes 1.19+
- Helm 3.8+
- CertWatch account with API key

## Installation

### Quick Start (Testing/Development)

For quick testing, provide the API key directly:

```bash
helm install cw-agent oci://ghcr.io/certwatch-app/helm-charts/cw-agent \
  --set agent.name=my-cluster-agent \
  --set apiKey.value=cw_your_api_key_here \
  --set certificates[0].hostname=api.example.com \
  --set certificates[0].port=443
```

> **Warning:** When using `apiKey.value`, the key is stored in Helm release history.
> Use `apiKey.existingSecret` for production deployments.

### Production Installation (Recommended)

For production, create a Kubernetes Secret first:

```bash
# Step 1: Create the secret
kubectl create secret generic cw-agent-api-key \
  --from-literal=api-key=cw_your_api_key_here

# Step 2: Install the chart referencing the secret
helm install cw-agent oci://ghcr.io/certwatch-app/helm-charts/cw-agent \
  --set agent.name=my-cluster-agent \
  --set apiKey.existingSecret.name=cw-agent-api-key \
  --set certificates[0].hostname=api.example.com \
  --set certificates[0].port=443
```

## Configuration

See [values.yaml](values.yaml) for all configuration options.

### Key Parameters

| Parameter | Description | Default |
|-----------|-------------|---------|
| `agent.name` | Unique agent name (required) | `""` |
| `agent.syncInterval` | Sync frequency | `5m` |
| `agent.scanInterval` | Scan frequency | `1m` |
| `agent.metricsPort` | Prometheus metrics port (0 to disable) | `8080` |
| `agent.heartbeatInterval` | Heartbeat interval (0 to disable) | `30s` |
| `apiKey.value` | API key value (creates Secret, not for production) | `""` |
| `apiKey.existingSecret.name` | Name of existing Secret with API key | `""` |
| `apiKey.existingSecret.key` | Key in the Secret containing API key | `api-key` |
| `certificates` | List of certificates to monitor | `[]` |

### API Key Configuration

You must provide the API key using **one** of these methods:

| Method | Parameter | Use Case |
|--------|-----------|----------|
| Direct value | `apiKey.value` | Testing/development only |
| Existing Secret | `apiKey.existingSecret.name` | Production (recommended) |

### Example values.yaml

```yaml
agent:
  name: production-cluster
  syncInterval: 10m

certificates:
  - hostname: api.mycompany.com
    port: 443
    tags:
      - production
      - api
  - hostname: web.mycompany.com
    port: 443
    tags:
      - production
      - web

serviceMonitor:
  enabled: true
```

Install with custom values:

```bash
helm install cw-agent oci://ghcr.io/certwatch-app/helm-charts/cw-agent -f my-values.yaml
```

### Using External ConfigMap

For complex deployments, use an existing ConfigMap containing the full certwatch.yaml:

```yaml
existingConfigMap:
  name: my-certwatch-config
  key: certwatch.yaml
```

## ArgoCD / FluxCD

Both GitOps tools support OCI registries natively.

### ArgoCD

```yaml
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: cw-agent
spec:
  project: default
  source:
    repoURL: ghcr.io/certwatch-app/helm-charts
    chart: cw-agent
    targetRevision: 0.4.0
    helm:
      values: |
        agent:
          name: production-cluster
        certificates:
          - hostname: api.example.com
            port: 443
  destination:
    server: https://kubernetes.default.svc
    namespace: monitoring
```

### FluxCD

```yaml
apiVersion: source.toolkit.fluxcd.io/v1
kind: OCIRepository
metadata:
  name: cw-agent
  namespace: flux-system
spec:
  interval: 5m
  url: oci://ghcr.io/certwatch-app/helm-charts/cw-agent
  ref:
    tag: 0.4.0
---
apiVersion: helm.toolkit.fluxcd.io/v2
kind: HelmRelease
metadata:
  name: cw-agent
  namespace: monitoring
spec:
  interval: 5m
  chartRef:
    kind: OCIRepository
    name: cw-agent
    namespace: flux-system
  values:
    agent:
      name: production-cluster
    certificates:
      - hostname: api.example.com
        port: 443
```

## Prometheus Integration

### Enable ServiceMonitor

If you're using Prometheus Operator:

```yaml
serviceMonitor:
  enabled: true
  interval: 30s
  labels:
    release: prometheus
```

### Available Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `certwatch_certificate_days_until_expiry` | Gauge | Days until certificate expires |
| `certwatch_certificate_valid` | Gauge | Certificate validity (1=valid, 0=invalid) |
| `certwatch_certificate_chain_valid` | Gauge | Chain validity (1=valid, 0=invalid) |
| `certwatch_scan_total` | Counter | Total scans by status |
| `certwatch_sync_total` | Counter | Total syncs by status |

## Health Endpoints

When metrics are enabled (default), the following endpoints are available:

| Endpoint | Description |
|----------|-------------|
| `/healthz` | Basic liveness check |
| `/readyz` | Readiness probe - 503 during init |
| `/livez` | Deep liveness - 503 if no recent scans |

## Upgrading

```bash
helm upgrade cw-agent oci://ghcr.io/certwatch-app/helm-charts/cw-agent
```

## Uninstalling

```bash
helm uninstall cw-agent
kubectl delete secret cw-agent-api-key  # Optional: remove API key secret
```

## Documentation

- [Full Agent Documentation](https://docs.certwatch.app/agent)
- [GitHub Repository](https://github.com/certwatch-app/cw-agent)
- [CertWatch Dashboard](https://certwatch.app)

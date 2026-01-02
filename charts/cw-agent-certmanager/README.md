# CertWatch Agent for cert-manager

[![Artifact Hub](https://img.shields.io/endpoint?url=https://artifacthub.io/badge/repository/cw-agent)](https://artifacthub.io/packages/search?repo=cw-agent)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](../../LICENSE)

Helm chart for deploying the CertWatch cert-manager controller. This agent monitors certificates managed by [cert-manager](https://cert-manager.io/) in your Kubernetes cluster and syncs them to [CertWatch](https://certwatch.app).

> **Tip:** For deploying multiple CertWatch agents, consider using the [cw-stack](../cw-stack/README.md) umbrella chart.

## Prerequisites

- Kubernetes 1.19+
- Helm 3.8+
- cert-manager installed in your cluster
- CertWatch account and API key

## Installation

### Quick Start

```bash
helm install cw-agent-certmanager oci://ghcr.io/certwatch-app/helm-charts/cw-agent-certmanager \
  --namespace certwatch --create-namespace \
  --set agent.name=my-cluster \
  --set apiKey.value=cw_your_api_key
```

### Using an Existing Secret (Recommended for Production)

```bash
# Create the secret first
kubectl create namespace certwatch
kubectl create secret generic cw-api-key \
  --namespace certwatch \
  --from-literal=api-key=cw_your_api_key

# Install the chart
helm install cw-agent-certmanager oci://ghcr.io/certwatch-app/helm-charts/cw-agent-certmanager \
  --namespace certwatch \
  --set agent.name=my-cluster \
  --set apiKey.existingSecret.name=cw-api-key
```

## Configuration

### Required Values

| Parameter | Description |
|-----------|-------------|
| `agent.name` | Unique name identifying this agent in CertWatch |
| `apiKey.value` OR `apiKey.existingSecret.name` | CertWatch API key |

### Key Configuration Options

| Parameter | Description | Default |
|-----------|-------------|---------|
| `agent.name` | Agent name (required) | `""` |
| `agent.logLevel` | Log level: debug, info, warn, error | `"info"` |
| `agent.syncInterval` | How often to sync certificates | `"30s"` |
| `agent.heartbeatInterval` | Heartbeat interval for offline alerts | `"30s"` |
| `agent.watchAllNamespaces` | Watch all namespaces | `true` |
| `agent.namespaces` | Specific namespaces to watch | `[]` |
| `agent.metricsPort` | Prometheus metrics port (0 to disable) | `9402` |
| `agent.healthPort` | Health probe port | `9403` |

### API Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `api.endpoint` | CertWatch API endpoint | `"https://api.certwatch.app"` |
| `api.timeout` | API request timeout | `"30s"` |
| `apiKey.value` | API key (creates a Secret) | `""` |
| `apiKey.existingSecret.name` | Name of existing Secret | `""` |
| `apiKey.existingSecret.key` | Key in the Secret | `"api-key"` |

### Resources & Security

| Parameter | Description | Default |
|-----------|-------------|---------|
| `resources.requests.cpu` | CPU request | `"10m"` |
| `resources.requests.memory` | Memory request | `"64Mi"` |
| `resources.limits.cpu` | CPU limit | `"100m"` |
| `resources.limits.memory` | Memory limit | `"128Mi"` |
| `securityContext.runAsNonRoot` | Run as non-root | `true` |
| `securityContext.readOnlyRootFilesystem` | Read-only filesystem | `true` |

### Observability

| Parameter | Description | Default |
|-----------|-------------|---------|
| `serviceMonitor.enabled` | Create Prometheus ServiceMonitor | `false` |
| `serviceMonitor.interval` | Scrape interval | `"30s"` |
| `podDisruptionBudget.enabled` | Create PodDisruptionBudget | `false` |

## Example Values File

```yaml
agent:
  name: production-cluster
  logLevel: info
  watchAllNamespaces: true

apiKey:
  existingSecret:
    name: cw-api-key

resources:
  requests:
    cpu: 10m
    memory: 64Mi
  limits:
    cpu: 100m
    memory: 128Mi

serviceMonitor:
  enabled: true
  interval: 30s

podDisruptionBudget:
  enabled: true
  minAvailable: 1
```

## RBAC

This chart creates a ClusterRole with permissions to:
- **Read** cert-manager resources: certificates, certificaterequests, issuers, clusterissuers
- **Read** core resources: secrets (certificate data), events, namespaces

## Upgrading

```bash
helm upgrade cw-agent-certmanager oci://ghcr.io/certwatch-app/helm-charts/cw-agent-certmanager \
  --namespace certwatch \
  --reuse-values
```

## Uninstalling

```bash
helm uninstall cw-agent-certmanager --namespace certwatch
```

Note: The API key Secret has `helm.sh/resource-policy: keep` and will not be deleted automatically.

## Troubleshooting

### View Logs
```bash
kubectl logs -n certwatch -l app.kubernetes.io/name=cw-agent-certmanager -f
```

### Check Agent Status
```bash
kubectl get pods -n certwatch -l app.kubernetes.io/name=cw-agent-certmanager
```

### Verify RBAC
```bash
kubectl auth can-i list certificates.cert-manager.io \
  --as=system:serviceaccount:certwatch:cw-agent-certmanager
```

## Related Charts

- [cw-agent](../cw-agent/README.md) - Network certificate scanner
- [cw-stack](../cw-stack/README.md) - Umbrella chart for deploying both agents

## Documentation

- [CertWatch Documentation](https://docs.certwatch.app)
- [cert-manager Integration Guide](https://docs.certwatch.app/agent-certmanager)
- [GitHub Repository](https://github.com/certwatch-app/cw-agent)

## License

Apache 2.0

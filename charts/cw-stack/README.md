# CertWatch Stack

[![Artifact Hub](https://img.shields.io/endpoint?url=https://artifacthub.io/badge/repository/cw-agent)](https://artifacthub.io/packages/search?repo=cw-agent)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](../../LICENSE)

Umbrella Helm chart for deploying CertWatch agents. Deploy one or both agents with a single chart, similar to [kube-prometheus-stack](https://github.com/prometheus-community/helm-charts/tree/main/charts/kube-prometheus-stack).

## Included Components

| Component | Description | Enable Flag |
|-----------|-------------|-------------|
| **cw-agent** | Network scanner for monitoring TLS certificates from endpoints | `agent.enabled` |
| **cw-agent-certmanager** | Controller for monitoring cert-manager certificates | `certManager.enabled` |

## Prerequisites

- Kubernetes 1.19+
- Helm 3.8+
- CertWatch account and API key
- cert-manager (if using cw-agent-certmanager)

## Installation

### Deploy Both Agents

```bash
helm install certwatch oci://ghcr.io/certwatch-app/helm-charts/cw-stack \
  --namespace certwatch --create-namespace \
  --set global.apiKey.value=cw_your_api_key \
  --set agent.enabled=true \
  --set certManager.enabled=true \
  --set cw-agent.agent.name=network-scanner \
  --set cw-agent-certmanager.agent.name=k8s-cluster
```

### Deploy Only cert-manager Agent

```bash
helm install certwatch oci://ghcr.io/certwatch-app/helm-charts/cw-stack \
  --namespace certwatch --create-namespace \
  --set global.apiKey.value=cw_your_api_key \
  --set certManager.enabled=true \
  --set cw-agent-certmanager.agent.name=production
```

### Deploy Only Network Scanner

```bash
helm install certwatch oci://ghcr.io/certwatch-app/helm-charts/cw-stack \
  --namespace certwatch --create-namespace \
  --set global.apiKey.value=cw_your_api_key \
  --set agent.enabled=true \
  --set cw-agent.agent.name=scanner
```

### Using Existing Secret (Recommended)

```bash
# Create secret first
kubectl create namespace certwatch
kubectl create secret generic cw-api-key \
  --namespace certwatch \
  --from-literal=api-key=cw_your_api_key

# Install with existing secret
helm install certwatch oci://ghcr.io/certwatch-app/helm-charts/cw-stack \
  --namespace certwatch \
  --set global.apiKey.existingSecret.name=cw-api-key \
  --set agent.enabled=true \
  --set certManager.enabled=true \
  --set cw-agent.agent.name=network-scanner \
  --set cw-agent-certmanager.agent.name=k8s-cluster
```

## Configuration

### Global Values

These values are shared by all subcharts:

| Parameter | Description | Default |
|-----------|-------------|---------|
| `global.api.endpoint` | CertWatch API endpoint | `"https://api.certwatch.app"` |
| `global.api.timeout` | API request timeout | `"30s"` |
| `global.apiKey.value` | Shared API key (creates Secrets) | `""` |
| `global.apiKey.existingSecret.name` | Existing Secret name | `""` |
| `global.apiKey.existingSecret.key` | Key in the Secret | `"api-key"` |

### Enable Flags

| Parameter | Description | Default |
|-----------|-------------|---------|
| `agent.enabled` | Deploy cw-agent (network scanner) | `false` |
| `certManager.enabled` | Deploy cw-agent-certmanager | `false` |

### Subchart Configuration

Configure each subchart using its alias as the key:

```yaml
# cw-agent configuration
cw-agent:
  agent:
    name: network-scanner
    logLevel: info
  # Override global API key for this agent only
  # apiKey:
  #   value: cw_different_key

# cw-agent-certmanager configuration
cw-agent-certmanager:
  agent:
    name: k8s-cluster
    watchAllNamespaces: true
```

See individual chart READMEs for all options:
- [cw-agent values](../cw-agent/README.md)
- [cw-agent-certmanager values](../cw-agent-certmanager/README.md)

## Example Values File

```yaml
global:
  apiKey:
    existingSecret:
      name: cw-api-key

agent:
  enabled: true

certManager:
  enabled: true

cw-agent:
  agent:
    name: network-scanner
    logLevel: info
  certificates:
    - hostname: api.example.com
      port: 443
    - hostname: www.example.com
      port: 443
  resources:
    requests:
      cpu: 10m
      memory: 32Mi

cw-agent-certmanager:
  agent:
    name: k8s-cluster
    watchAllNamespaces: true
  serviceMonitor:
    enabled: true
  podDisruptionBudget:
    enabled: true
    minAvailable: 1
```

## API Key Override

Each agent can use a different API key by setting it at the subchart level:

```yaml
global:
  apiKey:
    value: cw_default_key  # Used by default

cw-agent:
  apiKey:
    value: cw_scanner_specific_key  # Overrides global for cw-agent

cw-agent-certmanager:
  apiKey:
    existingSecret:
      name: different-secret  # Overrides global for cw-agent-certmanager
```

## Upgrading

```bash
helm upgrade certwatch oci://ghcr.io/certwatch-app/helm-charts/cw-stack \
  --namespace certwatch \
  --reuse-values
```

## Uninstalling

```bash
helm uninstall certwatch --namespace certwatch
```

## Troubleshooting

### View All CertWatch Pods
```bash
kubectl get pods -n certwatch -l app.kubernetes.io/part-of=certwatch
```

### View Logs
```bash
# Network scanner logs
kubectl logs -n certwatch -l app.kubernetes.io/name=cw-agent -f

# cert-manager agent logs
kubectl logs -n certwatch -l app.kubernetes.io/name=cw-agent-certmanager -f
```

### Check Enabled Components
```bash
helm get values certwatch -n certwatch
```

## Migrating from Individual Charts

If you previously installed `cw-agent` standalone:

```bash
# Uninstall old release
helm uninstall cw-agent -n certwatch

# Install via cw-stack
helm install certwatch oci://ghcr.io/certwatch-app/helm-charts/cw-stack \
  --namespace certwatch \
  --set agent.enabled=true \
  --set cw-agent.agent.name=your-existing-name \
  ...
```

## Individual Charts

For standalone deployments, you can install the charts individually:

- [cw-agent](../cw-agent/README.md) - Network certificate scanner
- [cw-agent-certmanager](../cw-agent-certmanager/README.md) - cert-manager controller

## Documentation

- [CertWatch Documentation](https://docs.certwatch.app)
- [cw-agent Guide](https://docs.certwatch.app/agent)
- [cert-manager Integration](https://docs.certwatch.app/agent-certmanager)
- [GitHub Repository](https://github.com/certwatch-app/cw-agent)

## License

Apache 2.0

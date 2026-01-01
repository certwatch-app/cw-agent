# cert-manager Integration

Monitor certificates managed by [cert-manager](https://cert-manager.io/) in your Kubernetes cluster.

## Overview

The `cw-agent-certmanager` controller watches for Certificate resources in your cluster and syncs their status to CertWatch. This gives you:

- **Unified dashboard** - See all certificates (network + Kubernetes) in one place
- **Expiry alerts** - Get notified before certificates expire
- **Audit trail** - Track certificate renewals and changes
- **Multi-cluster view** - Monitor certificates across all your clusters

## How It Works

```
┌─────────────────────────────────────────────────────────────┐
│                    Kubernetes Cluster                        │
│                                                              │
│  ┌──────────────┐    watches    ┌─────────────────────────┐ │
│  │ cert-manager │──────────────▶│     Certificates        │ │
│  │  (issuer)    │    creates    │  (cert-manager CRDs)    │ │
│  └──────────────┘               └───────────┬─────────────┘ │
│                                             │               │
│                                      watches│               │
│                                             ▼               │
│                               ┌─────────────────────────┐   │
│                               │  cw-agent-certmanager   │   │
│                               │     (controller)        │   │
│                               └───────────┬─────────────┘   │
│                                           │                 │
└───────────────────────────────────────────┼─────────────────┘
                                            │ syncs
                                            ▼
                               ┌─────────────────────────┐
                               │   CertWatch Cloud       │
                               │   (Dashboard & Alerts)  │
                               └─────────────────────────┘
```

The controller:
1. Watches Certificate resources in specified namespaces (or all namespaces)
2. Reads certificate data from the associated Secrets
3. Extracts expiry, issuer, and other metadata
4. Syncs to CertWatch API on a configurable interval

## Prerequisites

- Kubernetes 1.19+
- [cert-manager](https://cert-manager.io/docs/installation/) installed
- CertWatch account with API key

## Installation

### Quick Start

```bash
helm install cw-agent-certmanager oci://ghcr.io/certwatch-app/helm-charts/cw-agent-certmanager \
  --namespace certwatch --create-namespace \
  --set agent.name=my-cluster \
  --set apiKey.value=cw_your_api_key
```

### Production (with cw-stack)

```bash
# Create secret
kubectl create namespace certwatch
kubectl create secret generic cw-api-key \
  --namespace certwatch \
  --from-literal=api-key=cw_your_api_key

# Install
helm install certwatch oci://ghcr.io/certwatch-app/helm-charts/cw-stack \
  --namespace certwatch \
  --set global.apiKey.existingSecret.name=cw-api-key \
  --set certManager.enabled=true \
  --set cw-agent-certmanager.agent.name=production-cluster
```

## Configuration

### Watch All Namespaces (Default)

By default, the controller watches all namespaces:

```yaml
cw-agent-certmanager:
  agent:
    name: my-cluster
    watchAllNamespaces: true
```

### Watch Specific Namespaces

To limit to specific namespaces:

```yaml
cw-agent-certmanager:
  agent:
    name: my-cluster
    watchAllNamespaces: false
    namespaces:
      - production
      - staging
```

### Full Configuration

```yaml
cw-agent-certmanager:
  agent:
    name: production-cluster      # Required: unique identifier
    logLevel: info                # debug, info, warn, error
    syncInterval: 30s             # How often to sync to cloud
    heartbeatInterval: 30s        # Agent offline detection
    watchAllNamespaces: true      # Watch all namespaces
    namespaces: []                # Specific namespaces (if watchAllNamespaces=false)
    metricsPort: 9402             # Prometheus metrics port
    healthPort: 9403              # Health probe port

  api:
    endpoint: "https://api.certwatch.app"
    timeout: "30s"

  apiKey:
    existingSecret:
      name: cw-api-key
      key: api-key

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

## RBAC Permissions

The controller requires read access to cert-manager resources. The Helm chart creates a ClusterRole with these permissions:

```yaml
rules:
  # cert-manager resources
  - apiGroups: ["cert-manager.io"]
    resources: ["certificates", "certificaterequests", "issuers", "clusterissuers"]
    verbs: ["get", "list", "watch"]

  # Secrets containing certificate data
  - apiGroups: [""]
    resources: ["secrets"]
    verbs: ["get", "list", "watch"]

  # Events and namespaces
  - apiGroups: [""]
    resources: ["events", "namespaces"]
    verbs: ["get", "list", "watch"]
```

## What Gets Synced

For each Certificate resource, the controller syncs:

| Field | Source |
|-------|--------|
| Subject | Certificate Secret (tls.crt) |
| Issuer | Certificate Secret (tls.crt) |
| Expiry | Certificate Secret (tls.crt) |
| DNS Names | Certificate spec + Secret |
| Status | Certificate status conditions |
| Namespace | Certificate metadata |
| Labels | Certificate metadata |

## Prometheus Metrics

When metrics are enabled, the following are exposed:

| Metric | Type | Description |
|--------|------|-------------|
| `certwatch_certificate_days_until_expiry` | Gauge | Days until certificate expires |
| `certwatch_certificate_valid` | Gauge | Certificate validity (1=valid) |
| `certwatch_certificates_watched` | Gauge | Number of certificates being watched |
| `certwatch_sync_total` | Counter | Total syncs by status |
| `certwatch_sync_duration_seconds` | Histogram | Sync duration |
| `certwatch_heartbeat_total` | Counter | Total heartbeats by status |

## Combining with Network Scanner

You can run both agents to monitor:
- **Network endpoints** (external TLS services)
- **Kubernetes certificates** (cert-manager managed)

```yaml
# certwatch-values.yaml
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
  certificates:
    - hostname: api.example.com
      port: 443
    - hostname: www.example.com
      port: 443

cw-agent-certmanager:
  agent:
    name: k8s-certificates
    watchAllNamespaces: true
```

Both will appear in your CertWatch dashboard with their respective agent names.

## Troubleshooting

### No certificates appearing

1. **Check the controller is running:**
   ```bash
   kubectl get pods -n certwatch -l app.kubernetes.io/name=cw-agent-certmanager
   ```

2. **Check logs:**
   ```bash
   kubectl logs -n certwatch -l app.kubernetes.io/name=cw-agent-certmanager -f
   ```

3. **Verify cert-manager certificates exist:**
   ```bash
   kubectl get certificates --all-namespaces
   ```

4. **Check RBAC permissions:**
   ```bash
   kubectl auth can-i list certificates.cert-manager.io \
     --as=system:serviceaccount:certwatch:cw-agent-certmanager
   ```

### Certificate shows wrong expiry

The controller reads the actual certificate from the Secret, not the Certificate spec. Ensure the Secret exists and contains valid certificate data:

```bash
kubectl get secret <certificate-secret-name> -n <namespace> -o jsonpath='{.data.tls\.crt}' | \
  base64 -d | openssl x509 -noout -dates
```

### Controller not watching specific namespace

If using `watchAllNamespaces: false`, ensure the namespace is in the `namespaces` list:

```yaml
agent:
  watchAllNamespaces: false
  namespaces:
    - production
    - staging
    - your-namespace  # Add this
```

## Further Reading

- [cert-manager Documentation](https://cert-manager.io/docs/)
- [CertWatch Documentation](https://docs.certwatch.app)
- [cw-agent-certmanager Helm Chart](../charts/cw-agent-certmanager/README.md)
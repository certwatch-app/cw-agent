# Kubernetes Deployment Guide

Deploy CertWatch agents to Kubernetes using Helm charts.

## Available Charts

| Chart | Description | Use Case |
|-------|-------------|----------|
| [cw-agent](../charts/cw-agent/README.md) | Network certificate scanner | Monitor external TLS endpoints |
| [cw-agent-certmanager](../charts/cw-agent-certmanager/README.md) | cert-manager controller | Monitor Kubernetes certificates |
| [cw-stack](../charts/cw-stack/README.md) | Umbrella chart | Deploy one or both agents together |

## Quick Start

### Using cw-stack (Recommended)

The `cw-stack` umbrella chart is the easiest way to deploy CertWatch to Kubernetes:

```bash
helm install certwatch oci://ghcr.io/certwatch-app/helm-charts/cw-stack \
  --namespace certwatch --create-namespace \
  --set global.apiKey.value=cw_your_api_key \
  --set agent.enabled=true \
  --set certManager.enabled=true \
  --set cw-agent.agent.name=network-scanner \
  --set cw-agent-certmanager.agent.name=k8s-cluster
```

### Individual Charts

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

## Production Deployment

### Step 1: Create API Key Secret

Never store API keys in Helm values for production. Create a Kubernetes Secret:

```bash
kubectl create namespace certwatch

kubectl create secret generic cw-api-key \
  --namespace certwatch \
  --from-literal=api-key=cw_your_api_key
```

### Step 2: Create Values File

Create `certwatch-values.yaml`:

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
    name: production-scanner
    logLevel: info
  certificates:
    - hostname: api.mycompany.com
      port: 443
      tags: [production, api]
    - hostname: web.mycompany.com
      port: 443
      tags: [production, web]
  resources:
    requests:
      cpu: 10m
      memory: 32Mi
    limits:
      cpu: 100m
      memory: 128Mi
  serviceMonitor:
    enabled: true

cw-agent-certmanager:
  agent:
    name: k8s-certificates
    watchAllNamespaces: true
  resources:
    requests:
      cpu: 10m
      memory: 64Mi
    limits:
      cpu: 100m
      memory: 128Mi
  serviceMonitor:
    enabled: true
  podDisruptionBudget:
    enabled: true
    minAvailable: 1
```

### Step 3: Install

```bash
helm install certwatch oci://ghcr.io/certwatch-app/helm-charts/cw-stack \
  --namespace certwatch \
  -f certwatch-values.yaml
```

## GitOps Deployment

### ArgoCD

```yaml
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: certwatch
  namespace: argocd
spec:
  project: default
  source:
    repoURL: ghcr.io/certwatch-app/helm-charts
    chart: cw-stack
    targetRevision: 0.5.0
    helm:
      valueFiles:
        - values.yaml
      values: |
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
            name: production-scanner
        cw-agent-certmanager:
          agent:
            name: k8s-certificates
  destination:
    server: https://kubernetes.default.svc
    namespace: certwatch
  syncPolicy:
    automated:
      prune: true
      selfHeal: true
    syncOptions:
      - CreateNamespace=true
```

### FluxCD

**OCIRepository:**

```yaml
apiVersion: source.toolkit.fluxcd.io/v1
kind: OCIRepository
metadata:
  name: certwatch
  namespace: flux-system
spec:
  interval: 5m
  url: oci://ghcr.io/certwatch-app/helm-charts/cw-stack
  ref:
    tag: 0.5.0
```

**HelmRelease:**

```yaml
apiVersion: helm.toolkit.fluxcd.io/v2
kind: HelmRelease
metadata:
  name: certwatch
  namespace: certwatch
spec:
  interval: 5m
  chartRef:
    kind: OCIRepository
    name: certwatch
    namespace: flux-system
  values:
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
        name: production-scanner
    cw-agent-certmanager:
      agent:
        name: k8s-certificates
```

## Multi-Cluster Setup

For monitoring multiple clusters, deploy an agent in each cluster with a unique name:

**Cluster 1 (Production):**

```yaml
cw-agent-certmanager:
  agent:
    name: prod-us-east-1
```

**Cluster 2 (Staging):**

```yaml
cw-agent-certmanager:
  agent:
    name: staging-us-west-2
```

All certificates will appear in your CertWatch dashboard, organized by agent name.

## Monitoring the Agents

### View Pods

```bash
kubectl get pods -n certwatch -l app.kubernetes.io/part-of=certwatch
```

### View Logs

```bash
# Network scanner
kubectl logs -n certwatch -l app.kubernetes.io/name=cw-agent -f

# cert-manager agent
kubectl logs -n certwatch -l app.kubernetes.io/name=cw-agent-certmanager -f
```

### Prometheus Metrics

If `serviceMonitor.enabled: true`, metrics are automatically scraped by Prometheus Operator.

Query examples:

```promql
# Days until certificate expiry
certwatch_certificate_days_until_expiry

# Certificates expiring in 30 days
certwatch_certificate_days_until_expiry < 30

# Scan success rate
rate(certwatch_scan_total{status="success"}[5m])
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

# Optional: Remove the namespace
kubectl delete namespace certwatch
```

Note: The API key Secret has `helm.sh/resource-policy: keep` and will not be deleted automatically.

## Troubleshooting

### Agent not starting

Check events:

```bash
kubectl describe pod -n certwatch -l app.kubernetes.io/name=cw-agent
```

### Permission denied errors (cert-manager agent)

Verify RBAC:

```bash
kubectl auth can-i list certificates.cert-manager.io \
  --as=system:serviceaccount:certwatch:cw-agent-certmanager
```

### Certificates not syncing

1. Check agent logs for errors
2. Verify API key is valid
3. Ensure network connectivity to `api.certwatch.app`

```bash
kubectl run curl --rm -it --image=curlimages/curl -- \
  curl -v https://api.certwatch.app/v1/health
```
# Metrics & Observability

CertWatch Agent exposes Prometheus metrics and health endpoints for monitoring and alerting.

## Prometheus Metrics

### Enabling Metrics

**CLI:**

```yaml
# certwatch.yaml
agent:
  metrics_port: 8080  # 0 to disable
```

**Kubernetes (Helm):**

```yaml
# values.yaml
agent:
  metricsPort: 8080

serviceMonitor:
  enabled: true  # For Prometheus Operator
```

### Available Metrics

#### Certificate Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `certwatch_certificate_days_until_expiry` | Gauge | hostname, port | Days until certificate expires |
| `certwatch_certificate_valid` | Gauge | hostname, port | Certificate validity (1=valid, 0=invalid) |
| `certwatch_certificate_chain_valid` | Gauge | hostname, port | Chain validity (1=valid, 0=invalid) |
| `certwatch_certificate_expiry_timestamp_seconds` | Gauge | hostname, port | Expiry as Unix timestamp |

#### Scan Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `certwatch_scan_total` | Counter | status | Total scans (success/failure) |
| `certwatch_scan_duration_seconds` | Histogram | - | Scan duration distribution |

#### Sync Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `certwatch_sync_total` | Counter | status | Total syncs (success/failure) |
| `certwatch_sync_duration_seconds` | Histogram | - | Sync duration distribution |

#### Heartbeat Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `certwatch_heartbeat_total` | Counter | status | Total heartbeats (success/failure) |

#### Agent Info Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `certwatch_agent_info` | Gauge | version, name, agent_id | Agent information |
| `certwatch_agent_certificates_configured` | Gauge | - | Number of configured certificates |

### Example Queries

**Certificates expiring within 30 days:**

```promql
certwatch_certificate_days_until_expiry < 30
```

**Certificates expiring within 7 days (critical):**

```promql
certwatch_certificate_days_until_expiry < 7
```

**Invalid certificates:**

```promql
certwatch_certificate_valid == 0
```

**Scan success rate (last 5 minutes):**

```promql
rate(certwatch_scan_total{status="success"}[5m]) /
rate(certwatch_scan_total[5m])
```

**Sync failures:**

```promql
increase(certwatch_sync_total{status="failure"}[1h])
```

**Average scan duration:**

```promql
rate(certwatch_scan_duration_seconds_sum[5m]) /
rate(certwatch_scan_duration_seconds_count[5m])
```

### Alerting Rules

Example Prometheus alerting rules:

```yaml
groups:
  - name: certwatch
    rules:
      # Certificate expiring soon
      - alert: CertificateExpiringSoon
        expr: certwatch_certificate_days_until_expiry < 30
        for: 1h
        labels:
          severity: warning
        annotations:
          summary: "Certificate expiring soon"
          description: "Certificate for {{ $labels.hostname }}:{{ $labels.port }} expires in {{ $value }} days"

      # Certificate expiring critical
      - alert: CertificateExpiringCritical
        expr: certwatch_certificate_days_until_expiry < 7
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: "Certificate expiring critically soon"
          description: "Certificate for {{ $labels.hostname }}:{{ $labels.port }} expires in {{ $value }} days"

      # Invalid certificate
      - alert: CertificateInvalid
        expr: certwatch_certificate_valid == 0
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: "Invalid certificate detected"
          description: "Certificate for {{ $labels.hostname }}:{{ $labels.port }} is invalid"

      # Agent not syncing
      - alert: CertWatchSyncFailing
        expr: increase(certwatch_sync_total{status="failure"}[10m]) > 3
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "CertWatch sync failing"
          description: "CertWatch agent has failed to sync {{ $value }} times in the last 10 minutes"
```

### Grafana Dashboard

A sample Grafana dashboard JSON is available at:
- [grafana-dashboard.json](../examples/grafana-dashboard.json) (coming soon)

## Health Endpoints

When metrics are enabled, health endpoints are exposed on the same port.

### Endpoints

| Endpoint | Description | Success | Failure |
|----------|-------------|---------|---------|
| `/healthz` | Basic liveness | 200 OK | - |
| `/readyz` | Readiness probe | 200 OK | 503 during init |
| `/livez` | Deep liveness | 200 OK | 503 if no scans in 10min |
| `/metrics` | Prometheus metrics | 200 OK | - |

### Kubernetes Probes

The Helm chart configures probes automatically. Manual configuration:

```yaml
livenessProbe:
  httpGet:
    path: /livez
    port: 8080
  initialDelaySeconds: 30
  periodSeconds: 10
  failureThreshold: 3

readinessProbe:
  httpGet:
    path: /readyz
    port: 8080
  initialDelaySeconds: 5
  periodSeconds: 5
  failureThreshold: 3
```

### Health Check Behavior

**`/healthz`** - Always returns 200 if the HTTP server is running.

**`/readyz`** - Returns 503 during startup until:
- Configuration is loaded
- First scan completes
- First sync completes

**`/livez`** - Returns 503 if:
- No successful scan in the last 10 minutes
- Agent is in a degraded state

## Heartbeat & Offline Alerts

### How It Works

When `heartbeat_interval` is configured, the agent sends periodic heartbeats to CertWatch. If heartbeats stop, CertWatch can alert you that an agent is offline.

**Configuration:**

```yaml
agent:
  heartbeat_interval: 30s  # 0 to disable
```

### Offline Detection

CertWatch considers an agent offline if no heartbeat is received within `3 × heartbeat_interval`. With the default 30s interval:

- Expected heartbeat: every 30 seconds
- Offline threshold: 90 seconds without heartbeat
- Alert: Sent via configured notification channels

### Use Cases

- **Infrastructure monitoring** - Know immediately if an agent goes down
- **Network issues** - Detect connectivity problems between agent and cloud
- **Deployment validation** - Confirm agents are running after deployments

## ServiceMonitor (Prometheus Operator)

If you're using Prometheus Operator, enable ServiceMonitor:

```yaml
serviceMonitor:
  enabled: true
  interval: 30s
  scrapeTimeout: 10s
  labels:
    release: prometheus  # Match your Prometheus selector
  namespaceSelector:
    matchNames:
      - certwatch
```

The Helm chart creates a ServiceMonitor that automatically configures Prometheus to scrape CertWatch metrics.

## Logging

### Log Levels

| Level | Description |
|-------|-------------|
| `debug` | Verbose debugging information |
| `info` | Normal operational messages |
| `warn` | Warning conditions |
| `error` | Error conditions |

### Configuration

**CLI:**

```yaml
agent:
  log_level: info
```

**Kubernetes:**

```yaml
agent:
  logLevel: info
```

### Log Format

Logs are output in structured JSON format:

```json
{"level":"info","ts":"2024-01-15T10:30:00Z","msg":"scan completed","hostname":"example.com","port":443,"days_until_expiry":45}
```
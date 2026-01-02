# Architecture

Overview of CertWatch Agent architecture and how components work together.

## System Overview

```mermaid
flowchart TB
    subgraph infra["Your Infrastructure"]
        subgraph endpoints["Network Endpoints"]
            ep1["api.example.com:443"]
            ep2["web.example.com:443"]
            ep3["internal.example.com:443"]
        end

        subgraph k8s["Kubernetes Cluster"]
            cm["cert-manager"]
            certs["Certificate CRDs"]
            secrets["TLS Secrets"]

            cm -->|issues| certs
            certs -->|stores in| secrets
        end
    end

    subgraph agents["CertWatch Agents"]
        cwa["cw-agent<br/>(Network Scanner)"]
        cwacm["cw-agent-certmanager<br/>(K8s Controller)"]
    end

    subgraph cloud["CertWatch Cloud"]
        api["API Gateway"]
        db[(Database)]
        alerts["Alert Engine"]
        dash["Dashboard"]

        api --> db
        db --> alerts
        db --> dash
    end

    endpoints -->|TLS handshake| cwa
    secrets -->|watches| cwacm

    cwa -->|sync certificates| api
    cwacm -->|sync certificates| api

    alerts -->|notifications| notify["Email / Slack / Webhook"]
```

## Components

### cw-agent (Network Scanner)

The network scanner connects to TLS endpoints and extracts certificate information.

**How it works:**

1. **Configuration** - Reads list of hostnames/ports from config file
2. **Scanning** - Performs TLS handshake to each endpoint
3. **Extraction** - Parses certificate chain (subject, issuer, expiry, SANs)
4. **Validation** - Checks chain validity, expiration, weak crypto
5. **Syncing** - Sends certificate data to CertWatch API

**Architecture:**

```mermaid
flowchart LR
    subgraph agent["cw-agent"]
        config["Config<br/>Loader"]
        scanner["TLS<br/>Scanner"]
        sync["API<br/>Sync"]
        metrics["Metrics<br/>Server"]
        state["State<br/>Manager"]
    end

    yaml["certwatch.yaml"] --> config
    config --> scanner
    scanner --> sync
    sync --> api["CertWatch API"]
    state --> sync
    metrics --> prom["Prometheus"]
```

**Key features:**

- Concurrent scanning (configurable concurrency)
- Automatic retry on transient failures
- Certificate chain validation
- State persistence for agent ID

### cw-agent-certmanager (Kubernetes Controller)

The cert-manager controller watches Kubernetes Certificate resources.

**How it works:**

1. **Watch** - Uses Kubernetes informers to watch Certificate CRDs
2. **Read Secrets** - Reads actual certificate data from TLS Secrets
3. **Extract** - Parses certificate details from Secret data
4. **Sync** - Sends certificate data to CertWatch API

**Architecture:**

```mermaid
flowchart LR
    subgraph controller["cw-agent-certmanager"]
        informer["Certificate<br/>Informer"]
        secretReader["Secret<br/>Reader"]
        sync["API<br/>Sync"]
        metrics["Metrics<br/>Server"]
    end

    certs["Certificate CRDs"] --> informer
    informer --> secretReader
    secrets["TLS Secrets"] --> secretReader
    secretReader --> sync
    sync --> api["CertWatch API"]
    metrics --> prom["Prometheus"]
```

**Key features:**

- Namespace filtering (all or specific namespaces)
- Real-time updates via informers
- RBAC-scoped access (read-only)
- Leader election for HA (planned)

### CertWatch Cloud

The cloud platform receives certificate data and provides monitoring.

**Components:**

| Component | Purpose |
|-----------|---------|
| API Gateway | Receives sync requests, authenticates agents |
| Database | Stores certificate data, agent state |
| Alert Engine | Evaluates expiry rules, sends notifications |
| Dashboard | Web UI for viewing certificates |

## Data Flow

### Certificate Sync Flow

```mermaid
sequenceDiagram
    participant Agent
    participant API as CertWatch API
    participant DB as Database
    participant Alerts as Alert Engine

    Agent->>API: POST /v1/sync (certificates)
    API->>API: Validate API key
    API->>DB: Upsert certificates
    DB-->>API: OK
    API-->>Agent: 200 OK

    DB->>Alerts: Certificate updated
    Alerts->>Alerts: Check expiry rules
    alt Certificate expiring
        Alerts->>Notify: Send alert
    end
```

### Heartbeat Flow

```mermaid
sequenceDiagram
    participant Agent
    participant API as CertWatch API
    participant DB as Database

    loop Every heartbeat_interval
        Agent->>API: POST /v1/heartbeat
        API->>DB: Update last_seen
        DB-->>API: OK
        API-->>Agent: 200 OK
    end

    Note over API,DB: If no heartbeat for 3x interval
    DB->>Alerts: Agent offline
    Alerts->>Notify: Send offline alert
```

## Deployment Patterns

### Single Agent (CLI)

For monitoring external endpoints from a single location:

```
┌─────────────────┐         ┌─────────────────┐
│   Your Server   │         │ CertWatch Cloud │
│                 │         │                 │
│  ┌───────────┐  │  sync   │  ┌───────────┐  │
│  │ cw-agent  │──┼────────▶│  │    API    │  │
│  └───────────┘  │         │  └───────────┘  │
│                 │         │                 │
└─────────────────┘         └─────────────────┘
```

### Kubernetes (Helm)

For monitoring Kubernetes certificates and network endpoints:

```
┌─────────────────────────────────────────────────┐
│              Kubernetes Cluster                  │
│                                                  │
│  ┌─────────────┐      ┌─────────────────────┐   │
│  │  cw-agent   │      │cw-agent-certmanager │   │
│  │  (scanner)  │      │   (controller)      │   │
│  └──────┬──────┘      └──────────┬──────────┘   │
│         │                        │              │
└─────────┼────────────────────────┼──────────────┘
          │                        │
          │         sync           │
          └──────────┬─────────────┘
                     ▼
          ┌─────────────────┐
          │ CertWatch Cloud │
          └─────────────────┘
```

### Multi-Cluster

For monitoring multiple Kubernetes clusters:

```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│   Cluster A     │    │   Cluster B     │    │   Cluster C     │
│  (Production)   │    │   (Staging)     │    │  (Development)  │
│                 │    │                 │    │                 │
│ ┌─────────────┐ │    │ ┌─────────────┐ │    │ ┌─────────────┐ │
│ │certmanager  │ │    │ │certmanager  │ │    │ │certmanager  │ │
│ │agent: prod  │ │    │ │agent: stg   │ │    │ │agent: dev   │ │
│ └──────┬──────┘ │    │ └──────┬──────┘ │    │ └──────┬──────┘ │
└────────┼────────┘    └────────┼────────┘    └────────┼────────┘
         │                      │                      │
         └──────────────────────┼──────────────────────┘
                                ▼
                    ┌─────────────────────┐
                    │   CertWatch Cloud   │
                    │  (unified view)     │
                    └─────────────────────┘
```

## Security Model

### Agent Authentication

- API keys authenticate agents to CertWatch
- Keys scoped to `cloud:sync` permission
- Keys can be rotated without agent restart (planned)

### Network Security

- All communication over HTTPS
- Certificate pinning available (planned)
- Agent → Cloud only (no inbound connections)

### Kubernetes RBAC

The cert-manager controller uses minimal permissions:

```yaml
rules:
  - apiGroups: ["cert-manager.io"]
    resources: ["certificates", "certificaterequests", "issuers", "clusterissuers"]
    verbs: ["get", "list", "watch"]  # Read-only

  - apiGroups: [""]
    resources: ["secrets"]
    verbs: ["get", "list", "watch"]  # Read-only
```

### Container Security

Both agents run with security hardening:

```yaml
securityContext:
  runAsNonRoot: true
  readOnlyRootFilesystem: true
  allowPrivilegeEscalation: false
  capabilities:
    drop: [ALL]
```

## High Availability

### Network Scanner

- Run multiple replicas for redundancy
- Each replica scans all configured endpoints
- CertWatch deduplicates on sync

### cert-manager Controller

- Single replica recommended (leader election planned)
- PodDisruptionBudget for controlled upgrades
- Quick restart on failure (stateless design)

## Performance

### Resource Usage

| Component | CPU (idle) | CPU (active) | Memory |
|-----------|------------|--------------|--------|
| cw-agent | <1m | 10-50m | 32-64Mi |
| cw-agent-certmanager | <5m | 10-50m | 64-128Mi |

### Scaling Considerations

| Certificates | Recommended Config |
|--------------|-------------------|
| <100 | Default settings |
| 100-1000 | Increase sync_interval to 10m |
| >1000 | Contact support for enterprise options |
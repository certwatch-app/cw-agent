# CLI Reference

Complete reference for all CertWatch Agent CLI commands.

## Commands

### `cw-agent init`

Interactive configuration wizard for creating a new configuration file.

```bash
cw-agent init [flags]
```

**Flags:**

| Flag | Description | Default |
|------|-------------|---------|
| `-o, --output` | Output path for config file | `certwatch.yaml` |
| `--non-interactive` | Skip prompts, use environment variables | `false` |

**Interactive Mode:**

```bash
cw-agent init
cw-agent init -o /etc/certwatch/certwatch.yaml
```

**Non-Interactive Mode (CI/Automation):**

```bash
CW_API_KEY=cw_xxx \
CW_AGENT_NAME=my-agent \
CW_CERTIFICATES=api.example.com,web.example.com \
  cw-agent init --non-interactive -o certwatch.yaml
```

**Environment Variables for Non-Interactive Mode:**

| Variable | Required | Description |
|----------|----------|-------------|
| `CW_API_KEY` | Yes | API key with `cloud:sync` scope |
| `CW_API_ENDPOINT` | No | API endpoint URL |
| `CW_AGENT_NAME` | No | Agent name |
| `CW_SYNC_INTERVAL` | No | Sync interval (e.g., `5m`) |
| `CW_SCAN_INTERVAL` | No | Scan interval (e.g., `1m`) |
| `CW_LOG_LEVEL` | No | Log level |
| `CW_CERTIFICATES` | Yes | Comma-separated hostnames |

---

### `cw-agent start`

Start the monitoring agent.

```bash
cw-agent start [flags]
```

**Flags:**

| Flag | Description | Default |
|------|-------------|---------|
| `-c, --config` | Path to config file | `certwatch.yaml` |
| `--reset-agent` | Reset agent state and create new agent | `false` |
| `-y, --yes` | Skip confirmation prompts | `false` |

**Examples:**

```bash
# Start with config file
cw-agent start -c certwatch.yaml

# Reset agent state (creates new agent, migrates certificates)
cw-agent start -c certwatch.yaml --reset-agent

# Skip confirmation prompts (for CI/automation)
cw-agent start -c certwatch.yaml --reset-agent --yes
```

**Agent State:**

The agent persists state in `.certwatch-state.json` alongside your config file. This enables:

- **Restart resilience** - Agent ID survives restarts
- **Name change detection** - Warns if you change `agent.name` in config
- **Certificate migration** - When resetting, certificates transfer to new agent

---

### `cw-agent validate`

Validate configuration file without starting the agent.

```bash
cw-agent validate [flags]
```

**Flags:**

| Flag | Description | Default |
|------|-------------|---------|
| `-c, --config` | Path to config file | `certwatch.yaml` |

**Example:**

```bash
cw-agent validate -c certwatch.yaml
```

---

### `cw-agent version`

Display version information.

```bash
cw-agent version
```

**Output:**

```
cw-agent version 0.5.0
  commit: abc1234
  built:  2024-01-15T10:30:00Z
```

## Configuration File Reference

### Complete Schema

```yaml
# API connection settings
api:
  endpoint: "https://api.certwatch.app"  # CertWatch API URL
  key: "cw_xxxxx"                        # API key (required)
  timeout: "30s"                         # HTTP request timeout

# Agent settings
agent:
  name: "my-agent"           # Unique name for this agent (required)
  sync_interval: "5m"        # How often to sync with cloud
  scan_interval: "1m"        # How often to scan certificates
  concurrency: 10            # Max concurrent scans
  log_level: "info"          # Log level: debug, info, warn, error
  metrics_port: 8080         # Prometheus metrics port (0 to disable)
  heartbeat_interval: "30s"  # Heartbeat interval (0 to disable)

# Certificates to monitor
certificates:
  - hostname: "example.com"  # Hostname to connect to (required)
    port: 443                # Port (default: 443)
    tags:                    # Tags for organization
      - production
      - api
    notes: "Main API"        # Notes about this certificate
```

### Field Reference

#### `api` Section

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `endpoint` | string | No | `https://api.certwatch.app` | CertWatch API URL |
| `key` | string | Yes | - | API key with `cloud:sync` scope |
| `timeout` | duration | No | `30s` | HTTP request timeout |

#### `agent` Section

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `name` | string | Yes | `default-agent` | Unique name for this agent |
| `sync_interval` | duration | No | `5m` | How often to sync with cloud |
| `scan_interval` | duration | No | `1m` | How often to scan certificates |
| `concurrency` | int | No | `10` | Max concurrent certificate scans |
| `log_level` | string | No | `info` | Log level: debug, info, warn, error |
| `metrics_port` | int | No | `8080` | Prometheus metrics port (0 to disable) |
| `heartbeat_interval` | duration | No | `30s` | Heartbeat interval for offline alerts (0 to disable) |

#### `certificates` Section

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `hostname` | string | Yes | - | Hostname to connect to |
| `port` | int | No | `443` | Port to connect to |
| `tags` | []string | No | `[]` | Tags for organization |
| `notes` | string | No | `""` | Notes about this certificate |

## Exit Codes

| Code | Description |
|------|-------------|
| `0` | Success |
| `1` | General error |
| `2` | Configuration error |
| `3` | API connection error |

## Signals

The agent handles the following signals:

| Signal | Behavior |
|--------|----------|
| `SIGINT` (Ctrl+C) | Graceful shutdown |
| `SIGTERM` | Graceful shutdown |
| `SIGHUP` | Reload configuration (planned) |
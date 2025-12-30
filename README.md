# CertWatch Agent

[![CI](https://github.com/certwatch-app/cw-agent/actions/workflows/ci.yml/badge.svg)](https://github.com/certwatch-app/cw-agent/actions/workflows/ci.yml)
[![Release](https://github.com/certwatch-app/cw-agent/actions/workflows/release.yml/badge.svg)](https://github.com/certwatch-app/cw-agent/actions/workflows/release.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/certwatch-app/cw-agent)](https://goreportcard.com/report/github.com/certwatch-app/cw-agent)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)

SSL/TLS certificate monitoring agent for [CertWatch](https://certwaatch.app). Monitor certificates on your infrastructure and sync data to the CertWatch cloud platform.

## Features

- **Config-driven monitoring** - Define certificates to monitor in a YAML file
- **Automatic scanning** - Continuously scan certificates at configurable intervals
- **Cloud sync** - Automatically sync certificate data to CertWatch dashboard
- **Chain validation** - Detect chain issues, expiration, and weak cryptography
- **Lightweight** - Single binary, minimal resource usage
- **Secure** - Runs without root, distroless Docker image

## Quick Start

### 1. Install

**Using Go:**
```bash
go install github.com/certwatch-app/cw-agent/cmd/cw-agent@latest
```

**Using Homebrew (macOS/Linux):**
```bash
brew install certwatch-app/tap/cw-agent
```

**Using Docker:**
```bash
docker pull ghcr.io/certwatch-app/cw-agent:latest
```

**Download binary:**

Download the latest release from the [releases page](https://github.com/certwatch-app/cw-agent/releases).

### 2. Configure

Create a `certwatch.yaml` configuration file:

```yaml
api:
  endpoint: "https://api.certwaatch.app"
  key: "cw_xxxxxxxx_xxxx..."  # Get from CertWatch dashboard

agent:
  name: "production-monitor"
  sync_interval: 5m
  scan_interval: 1m

certificates:
  - hostname: "www.example.com"
    port: 443
    tags: ["production", "web"]

  - hostname: "api.example.com"
    port: 443
    tags: ["production", "api"]
```

See [certwatch.example.yaml](certwatch.example.yaml) for a complete example.

### 3. Run

```bash
cw-agent start -c certwatch.yaml
```

Or with Docker:

```bash
docker run -v $(pwd)/certwatch.yaml:/etc/certwatch/certwatch.yaml \
  ghcr.io/certwatch-app/cw-agent:latest
```

## Commands

```bash
# Start the agent
cw-agent start -c /path/to/certwatch.yaml

# Validate configuration
cw-agent validate -c /path/to/certwatch.yaml

# Show version
cw-agent version

# Get help
cw-agent --help
```

## Configuration

### API Settings

| Field | Description | Default |
|-------|-------------|---------|
| `api.endpoint` | CertWatch API URL | `https://api.certwaatch.app` |
| `api.key` | API key with `cloud:sync` scope | Required |
| `api.timeout` | HTTP request timeout | `30s` |

### Agent Settings

| Field | Description | Default |
|-------|-------------|---------|
| `agent.name` | Unique name for this agent | `default-agent` |
| `agent.sync_interval` | How often to sync with cloud | `5m` |
| `agent.scan_interval` | How often to scan certificates | `1m` |
| `agent.concurrency` | Max concurrent scans | `10` |
| `agent.log_level` | Log level (debug/info/warn/error) | `info` |

### Certificate Settings

| Field | Description | Default |
|-------|-------------|---------|
| `hostname` | Hostname to connect to | Required |
| `port` | Port to connect to | `443` |
| `tags` | Tags for organization | `[]` |
| `notes` | Notes about this certificate | `""` |

## Environment Variables

Configuration can also be set via environment variables with the `CW_` prefix:

```bash
export CW_API_KEY="cw_xxxx..."
export CW_AGENT_NAME="my-agent"
```

## Running as a Service

### systemd (Linux)

Create `/etc/systemd/system/cw-agent.service`:

```ini
[Unit]
Description=CertWatch Agent
After=network.target

[Service]
Type=simple
User=certwatch
ExecStart=/usr/local/bin/cw-agent start -c /etc/certwatch/certwatch.yaml
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
```

```bash
sudo systemctl enable cw-agent
sudo systemctl start cw-agent
```

### Docker Compose

```yaml
version: '3.8'
services:
  cw-agent:
    image: ghcr.io/certwatch-app/cw-agent:latest
    restart: unless-stopped
    volumes:
      - ./certwatch.yaml:/etc/certwatch/certwatch.yaml:ro
```

## Getting an API Key

1. Log in to [CertWatch](https://certwatch.app)
2. Go to **Settings** > **API Keys**
3. Create a new key with the `cloud:sync` scope
4. Copy the key (it's only shown once!)

## Development

### Building

```bash
# Build
make build

# Run tests
make test

# Run linter
make lint

# Build for all platforms
make build-all
```

### Project Structure

```
cw-agent/
├── cmd/cw-agent/       # Entry point
├── internal/
│   ├── agent/          # Main orchestrator
│   ├── cmd/            # CLI commands
│   ├── config/         # Configuration loading
│   ├── scanner/        # TLS certificate scanning
│   ├── sync/           # API client
│   └── version/        # Version info
├── certwatch.example.yaml
├── Dockerfile
├── Makefile
└── README.md
```

## Contributing

Contributions are welcome! Please read [CONTRIBUTING.md](CONTRIBUTING.md) for details.

## License

Apache 2.0 - see [LICENSE](LICENSE) for details.

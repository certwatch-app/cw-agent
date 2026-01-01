# Linux Deployment Guide

Run CertWatch Agent as a native Linux service using systemd for production certificate monitoring.

## Quick Start

```bash
# Install
curl -sSL https://certwatch.app/install.sh | bash

# Configure
cw-agent init

# Start as service
sudo systemctl enable --now cw-agent
```

## Installation

### Quick Install Script

The recommended way to install on Linux:

```bash
curl -sSL https://certwatch.app/install.sh | bash
```

This will:
- Download the latest release for your architecture
- Install to `/usr/local/bin/cw-agent`
- Create config directory at `/etc/certwatch/`

### Manual Installation

**Download from GitHub Releases:**

```bash
# For x86_64
VERSION="0.5.0"
curl -Lo cw-agent "https://github.com/certwatch-app/cw-agent/releases/download/v${VERSION}/cw-agent-linux-amd64"

# For ARM64
curl -Lo cw-agent "https://github.com/certwatch-app/cw-agent/releases/download/v${VERSION}/cw-agent-linux-arm64"

# Install
chmod +x cw-agent
sudo mv cw-agent /usr/local/bin/

# Verify
cw-agent version
```

**Using Go:**

```bash
go install github.com/certwatch-app/cw-agent/cmd/cw-agent@latest
```

### Package Managers

**Homebrew (Linux):**

```bash
brew install certwatch-app/tap/cw-agent
```

**APT (Debian/Ubuntu) - Coming Soon:**

```bash
# Add repository
curl -fsSL https://apt.certwatch.app/gpg | sudo gpg --dearmor -o /usr/share/keyrings/certwatch.gpg
echo "deb [signed-by=/usr/share/keyrings/certwatch.gpg] https://apt.certwatch.app stable main" | sudo tee /etc/apt/sources.list.d/certwatch.list

# Install
sudo apt update && sudo apt install cw-agent
```

## Configuration

### Interactive Setup

The easiest way to configure:

```bash
cw-agent init
```

This wizard guides you through:
1. API key configuration
2. Agent naming
3. Adding certificates to monitor
4. Metrics and heartbeat settings

### Manual Configuration

Create `/etc/certwatch/certwatch.yaml`:

```yaml
api:
  endpoint: "https://api.certwatch.app"
  key: "cw_your_api_key"
  timeout: 30s

agent:
  name: "linux-server"
  sync_interval: 5m
  scan_interval: 1m
  concurrency: 10
  log_level: info
  metrics_port: 8080
  heartbeat_interval: 30s

certificates:
  - hostname: "example.com"
    port: 443
    tags: ["production"]

  - hostname: "api.example.com"
    port: 443
    tags: ["production", "api"]
```

### Validate Configuration

```bash
cw-agent validate -c /etc/certwatch/certwatch.yaml
```

## Running as a Service

### systemd Service

**Create service file** (`/etc/systemd/system/cw-agent.service`):

```ini
[Unit]
Description=CertWatch Agent - SSL/TLS Certificate Monitoring
Documentation=https://docs.certwatch.app
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=certwatch
Group=certwatch
ExecStart=/usr/local/bin/cw-agent start -c /etc/certwatch/certwatch.yaml
Restart=always
RestartSec=10
StandardOutput=journal
StandardError=journal

# Security hardening
NoNewPrivileges=yes
ProtectSystem=strict
ProtectHome=yes
PrivateTmp=yes
PrivateDevices=yes
ProtectKernelTunables=yes
ProtectKernelModules=yes
ProtectControlGroups=yes
ReadOnlyPaths=/
ReadWritePaths=/var/lib/certwatch

# Resource limits
MemoryMax=128M
CPUQuota=50%

[Install]
WantedBy=multi-user.target
```

**Create user and directories:**

```bash
# Create service user
sudo useradd --system --no-create-home --shell /usr/sbin/nologin certwatch

# Create directories
sudo mkdir -p /etc/certwatch /var/lib/certwatch
sudo chown certwatch:certwatch /var/lib/certwatch

# Set config permissions
sudo chown root:certwatch /etc/certwatch/certwatch.yaml
sudo chmod 640 /etc/certwatch/certwatch.yaml
```

**Enable and start:**

```bash
sudo systemctl daemon-reload
sudo systemctl enable cw-agent
sudo systemctl start cw-agent
```

### Service Management

```bash
# Start/Stop/Restart
sudo systemctl start cw-agent
sudo systemctl stop cw-agent
sudo systemctl restart cw-agent

# Check status
sudo systemctl status cw-agent

# View logs
sudo journalctl -u cw-agent -f

# View recent logs
sudo journalctl -u cw-agent --since "1 hour ago"
```

## Multiple Agents

Run multiple agents on the same server for different environments:

### Create Separate Configs

```bash
# Production config
sudo cp /etc/certwatch/certwatch.yaml /etc/certwatch/certwatch-prod.yaml
# Edit with production settings...

# Staging config
sudo cp /etc/certwatch/certwatch.yaml /etc/certwatch/certwatch-staging.yaml
# Edit with staging settings...
```

### Create Service Instances

**Template service** (`/etc/systemd/system/cw-agent@.service`):

```ini
[Unit]
Description=CertWatch Agent (%i)
Documentation=https://docs.certwatch.app
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=certwatch
Group=certwatch
ExecStart=/usr/local/bin/cw-agent start -c /etc/certwatch/certwatch-%i.yaml
Restart=always
RestartSec=10
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
```

**Start instances:**

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now cw-agent@prod
sudo systemctl enable --now cw-agent@staging
```

**Manage instances:**

```bash
sudo systemctl status cw-agent@prod
sudo journalctl -u cw-agent@staging -f
```

## Prometheus Integration

### Enable Metrics

In `certwatch.yaml`:

```yaml
agent:
  metrics_port: 8080  # 0 to disable
```

### Prometheus Config

Add to `prometheus.yml`:

```yaml
scrape_configs:
  - job_name: 'cw-agent'
    static_configs:
      - targets: ['localhost:8080']
    relabel_configs:
      - source_labels: [__address__]
        target_label: instance
        replacement: 'linux-server'
```

### Firewall Rules

If Prometheus runs on a different server:

```bash
# UFW
sudo ufw allow from 10.0.0.0/8 to any port 8080

# firewalld
sudo firewall-cmd --add-port=8080/tcp --permanent
sudo firewall-cmd --reload

# iptables
sudo iptables -A INPUT -p tcp --dport 8080 -s 10.0.0.0/8 -j ACCEPT
```

## Security Best Practices

### File Permissions

```bash
# Binary
sudo chown root:root /usr/local/bin/cw-agent
sudo chmod 755 /usr/local/bin/cw-agent

# Config (contains API key)
sudo chown root:certwatch /etc/certwatch/certwatch.yaml
sudo chmod 640 /etc/certwatch/certwatch.yaml

# State directory
sudo chown certwatch:certwatch /var/lib/certwatch
sudo chmod 750 /var/lib/certwatch
```

### API Key from Environment

Instead of storing the API key in the config file:

```yaml
# certwatch.yaml
api:
  key: "${CW_API_KEY}"  # Read from environment
```

```ini
# In systemd service
[Service]
EnvironmentFile=/etc/certwatch/env
```

```bash
# /etc/certwatch/env
CW_API_KEY=cw_your_api_key
```

```bash
# Secure the env file
sudo chmod 600 /etc/certwatch/env
sudo chown root:root /etc/certwatch/env
```

### SELinux (RHEL/CentOS)

If SELinux is enforcing:

```bash
# Allow network connections
sudo setsebool -P httpd_can_network_connect 1

# Create policy for custom port
sudo semanage port -a -t http_port_t -p tcp 8080
```

## Monitoring & Alerting

### Log Rotation

Logs are handled by journald. Configure retention in `/etc/systemd/journald.conf`:

```ini
[Journal]
SystemMaxUse=500M
MaxRetentionSec=30day
```

### Health Check Script

Create `/usr/local/bin/cw-agent-healthcheck`:

```bash
#!/bin/bash
METRICS_PORT=${1:-8080}

if curl -sf "http://localhost:${METRICS_PORT}/healthz" > /dev/null; then
    echo "OK"
    exit 0
else
    echo "FAIL"
    exit 1
fi
```

```bash
sudo chmod +x /usr/local/bin/cw-agent-healthcheck
```

### Nagios/Icinga Check

```bash
/usr/local/bin/cw-agent-healthcheck 8080
```

### Cron-based Alerting

```bash
# /etc/cron.d/cw-agent-check
*/5 * * * * root /usr/local/bin/cw-agent-healthcheck || echo "CertWatch Agent down" | mail -s "Alert" admin@example.com
```

## Upgrading

### Manual Upgrade

```bash
# Stop service
sudo systemctl stop cw-agent

# Download new version
VERSION="0.5.1"
curl -Lo /tmp/cw-agent "https://github.com/certwatch-app/cw-agent/releases/download/v${VERSION}/cw-agent-linux-amd64"

# Replace binary
sudo mv /tmp/cw-agent /usr/local/bin/cw-agent
sudo chmod +x /usr/local/bin/cw-agent

# Start service
sudo systemctl start cw-agent

# Verify
cw-agent version
```

### Automated Upgrade Script

```bash
#!/bin/bash
# /usr/local/bin/cw-agent-upgrade

set -e

ARCH=$(uname -m)
case $ARCH in
    x86_64) ARCH="amd64" ;;
    aarch64) ARCH="arm64" ;;
esac

# Get latest version
LATEST=$(curl -s https://api.github.com/repos/certwatch-app/cw-agent/releases/latest | grep tag_name | cut -d '"' -f 4)

echo "Upgrading to $LATEST..."

# Download
curl -Lo /tmp/cw-agent "https://github.com/certwatch-app/cw-agent/releases/download/${LATEST}/cw-agent-linux-${ARCH}"

# Stop, upgrade, start
sudo systemctl stop cw-agent
sudo mv /tmp/cw-agent /usr/local/bin/cw-agent
sudo chmod +x /usr/local/bin/cw-agent
sudo systemctl start cw-agent

echo "Upgraded to $(cw-agent version)"
```

## Troubleshooting

### Service won't start

Check logs:

```bash
sudo journalctl -u cw-agent -n 50 --no-pager
```

Common issues:
- Invalid config file syntax
- Missing API key
- Permission denied on config file

### Permission denied

```bash
# Check file permissions
ls -la /etc/certwatch/certwatch.yaml

# Fix ownership
sudo chown root:certwatch /etc/certwatch/certwatch.yaml
sudo chmod 640 /etc/certwatch/certwatch.yaml
```

### Network connectivity

```bash
# Test API connectivity
curl -v https://api.certwatch.app/v1/health

# Test certificate endpoint
openssl s_client -connect example.com:443 -servername example.com < /dev/null
```

### High CPU usage

Reduce scan concurrency:

```yaml
agent:
  concurrency: 5  # Lower from default 10
  scan_interval: 2m  # Increase interval
```

### State file issues

If the agent reports state file errors:

```bash
# Check state directory
ls -la /var/lib/certwatch/

# Reset state (certificates will re-sync)
sudo rm /var/lib/certwatch/.certwatch-state.json
sudo systemctl restart cw-agent
```

## Uninstalling

```bash
# Stop and disable service
sudo systemctl stop cw-agent
sudo systemctl disable cw-agent

# Remove files
sudo rm /etc/systemd/system/cw-agent.service
sudo rm /usr/local/bin/cw-agent
sudo rm -rf /etc/certwatch
sudo rm -rf /var/lib/certwatch

# Remove user
sudo userdel certwatch

# Reload systemd
sudo systemctl daemon-reload
```

## Further Reading

- [CLI Reference](cli-reference.md) - All commands and options
- [Docker Guide](docker.md) - Container deployments
- [Metrics Reference](metrics.md) - Prometheus metrics
- [Getting Started](getting-started.md) - Quick start guide

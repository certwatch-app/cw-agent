# Docker Deployment Guide

Run CertWatch Agent as a Docker container for local or server-based certificate monitoring.

## Quick Start

```bash
# Create config file
cat > certwatch.yaml << 'EOF'
api:
  key: "cw_your_api_key"

agent:
  name: "docker-agent"

certificates:
  - hostname: "example.com"
    port: 443
EOF

# Run the agent
docker run -d \
  --name cw-agent \
  --restart unless-stopped \
  -v $(pwd)/certwatch.yaml:/etc/certwatch/certwatch.yaml:ro \
  ghcr.io/certwatch-app/cw-agent:latest
```

## Container Images

Images are published to GitHub Container Registry:

| Tag | Description |
|-----|-------------|
| `latest` | Latest stable release |
| `v0.5.0` | Specific version |
| `v0.5` | Latest patch for v0.5.x |
| `sha-abc1234` | Specific commit (for testing) |

```bash
# Pull specific version
docker pull ghcr.io/certwatch-app/cw-agent:v0.5.0
```

## Configuration Options

### Using Config File (Recommended)

Mount your configuration file:

```bash
docker run -d \
  --name cw-agent \
  -v /path/to/certwatch.yaml:/etc/certwatch/certwatch.yaml:ro \
  ghcr.io/certwatch-app/cw-agent:latest
```

### Using Environment Variables

For simple setups, use environment variables:

```bash
docker run -d \
  --name cw-agent \
  -e CW_API_KEY="cw_your_api_key" \
  -e CW_AGENT_NAME="docker-agent" \
  -e CW_CERTIFICATES="example.com:443,api.example.com:443" \
  ghcr.io/certwatch-app/cw-agent:latest
```

Available environment variables:

| Variable | Description | Default |
|----------|-------------|---------|
| `CW_API_KEY` | CertWatch API key (required) | - |
| `CW_AGENT_NAME` | Agent identifier | `cw-agent` |
| `CW_CERTIFICATES` | Comma-separated `host:port` pairs | - |
| `CW_API_ENDPOINT` | API endpoint URL | `https://api.certwatch.app` |
| `CW_SYNC_INTERVAL` | Sync interval | `5m` |
| `CW_SCAN_INTERVAL` | Scan interval | `1m` |
| `CW_LOG_LEVEL` | Log level | `info` |
| `CW_METRICS_PORT` | Metrics port (0 to disable) | `8080` |

## Docker Compose

### Basic Setup

```yaml
# docker-compose.yml
services:
  cw-agent:
    image: ghcr.io/certwatch-app/cw-agent:latest
    container_name: cw-agent
    restart: unless-stopped
    volumes:
      - ./certwatch.yaml:/etc/certwatch/certwatch.yaml:ro
```

### With Prometheus Metrics

```yaml
# docker-compose.yml
services:
  cw-agent:
    image: ghcr.io/certwatch-app/cw-agent:latest
    container_name: cw-agent
    restart: unless-stopped
    ports:
      - "8080:8080"  # Prometheus metrics
    volumes:
      - ./certwatch.yaml:/etc/certwatch/certwatch.yaml:ro
    healthcheck:
      test: ["CMD", "wget", "-q", "--spider", "http://localhost:8080/healthz"]
      interval: 30s
      timeout: 10s
      retries: 3

  prometheus:
    image: prom/prometheus:latest
    volumes:
      - ./prometheus.yml:/etc/prometheus/prometheus.yml:ro
    ports:
      - "9090:9090"
```

Prometheus config (`prometheus.yml`):

```yaml
scrape_configs:
  - job_name: 'cw-agent'
    static_configs:
      - targets: ['cw-agent:8080']
```

### Multiple Agents

Monitor different environments with separate agents:

```yaml
# docker-compose.yml
services:
  cw-agent-prod:
    image: ghcr.io/certwatch-app/cw-agent:latest
    container_name: cw-agent-prod
    restart: unless-stopped
    volumes:
      - ./certwatch-prod.yaml:/etc/certwatch/certwatch.yaml:ro

  cw-agent-staging:
    image: ghcr.io/certwatch-app/cw-agent:latest
    container_name: cw-agent-staging
    restart: unless-stopped
    volumes:
      - ./certwatch-staging.yaml:/etc/certwatch/certwatch.yaml:ro
```

## Production Configuration

### Full Config Example

```yaml
# certwatch.yaml
api:
  endpoint: "https://api.certwatch.app"
  key: "cw_your_api_key"
  timeout: 30s

agent:
  name: "production-docker"
  sync_interval: 5m
  scan_interval: 1m
  concurrency: 10
  log_level: info
  metrics_port: 8080
  heartbeat_interval: 30s

certificates:
  # Production services
  - hostname: "api.mycompany.com"
    port: 443
    tags: ["production", "api"]

  - hostname: "web.mycompany.com"
    port: 443
    tags: ["production", "web"]

  - hostname: "admin.mycompany.com"
    port: 443
    tags: ["production", "admin"]

  # External dependencies
  - hostname: "stripe.com"
    port: 443
    tags: ["external", "payments"]

  - hostname: "api.sendgrid.com"
    port: 443
    tags: ["external", "email"]
```

### Secrets Management

**Using Docker Secrets (Swarm):**

```yaml
# docker-compose.yml (Swarm mode)
services:
  cw-agent:
    image: ghcr.io/certwatch-app/cw-agent:latest
    secrets:
      - cw_api_key
    environment:
      - CW_API_KEY_FILE=/run/secrets/cw_api_key
    volumes:
      - ./certwatch.yaml:/etc/certwatch/certwatch.yaml:ro

secrets:
  cw_api_key:
    external: true
```

**Using .env file:**

```bash
# .env (DO NOT commit to git!)
CW_API_KEY=cw_your_api_key
```

```yaml
# docker-compose.yml
services:
  cw-agent:
    image: ghcr.io/certwatch-app/cw-agent:latest
    env_file:
      - .env
    volumes:
      - ./certwatch.yaml:/etc/certwatch/certwatch.yaml:ro
```

### Resource Limits

```yaml
services:
  cw-agent:
    image: ghcr.io/certwatch-app/cw-agent:latest
    deploy:
      resources:
        limits:
          cpus: '0.5'
          memory: 128M
        reservations:
          cpus: '0.1'
          memory: 32M
```

## Network Considerations

### Monitoring Internal Services

To monitor services on the Docker network:

```yaml
services:
  cw-agent:
    image: ghcr.io/certwatch-app/cw-agent:latest
    networks:
      - frontend
      - backend
    volumes:
      - ./certwatch.yaml:/etc/certwatch/certwatch.yaml:ro

  nginx:
    image: nginx:alpine
    networks:
      - frontend

networks:
  frontend:
  backend:
```

In your config, use container names:

```yaml
certificates:
  - hostname: "nginx"  # Docker container name
    port: 443
```

### Host Network Mode

To monitor services on the host:

```bash
docker run -d \
  --name cw-agent \
  --network host \
  -v $(pwd)/certwatch.yaml:/etc/certwatch/certwatch.yaml:ro \
  ghcr.io/certwatch-app/cw-agent:latest
```

## Monitoring & Logs

### View Logs

```bash
# Follow logs
docker logs -f cw-agent

# Last 100 lines
docker logs --tail 100 cw-agent

# With timestamps
docker logs -t cw-agent
```

### Health Checks

```bash
# Check container health
docker inspect --format='{{.State.Health.Status}}' cw-agent

# Manual health check
docker exec cw-agent wget -q --spider http://localhost:8080/healthz && echo "OK"
```

### Prometheus Metrics

Access metrics at `http://localhost:8080/metrics`:

```bash
curl http://localhost:8080/metrics | grep certwatch
```

## Upgrading

### Manual Upgrade

```bash
# Pull new image
docker pull ghcr.io/certwatch-app/cw-agent:latest

# Stop and remove old container
docker stop cw-agent && docker rm cw-agent

# Start with new image
docker run -d \
  --name cw-agent \
  --restart unless-stopped \
  -v $(pwd)/certwatch.yaml:/etc/certwatch/certwatch.yaml:ro \
  ghcr.io/certwatch-app/cw-agent:latest
```

### Docker Compose Upgrade

```bash
docker-compose pull
docker-compose up -d
```

### Watchtower (Automatic Updates)

```yaml
services:
  cw-agent:
    image: ghcr.io/certwatch-app/cw-agent:latest
    labels:
      - "com.centurylinklabs.watchtower.enable=true"

  watchtower:
    image: containrrr/watchtower
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
    command: --interval 86400 --cleanup
```

## Troubleshooting

### Container won't start

Check logs:

```bash
docker logs cw-agent
```

Common issues:
- Invalid config file path
- Missing API key
- Invalid YAML syntax

### Certificate scan failing

Verify network connectivity from container:

```bash
docker exec cw-agent wget -q --spider https://example.com && echo "OK"
```

### Metrics not available

Ensure the port is exposed:

```bash
docker run -d \
  --name cw-agent \
  -p 8080:8080 \
  -v $(pwd)/certwatch.yaml:/etc/certwatch/certwatch.yaml:ro \
  ghcr.io/certwatch-app/cw-agent:latest
```

### High memory usage

Reduce concurrency in config:

```yaml
agent:
  concurrency: 5  # Lower from default 10
```

## Further Reading

- [CLI Reference](cli-reference.md) - All configuration options
- [Metrics Reference](metrics.md) - Prometheus metrics and alerting
- [Getting Started](getting-started.md) - Quick start guide

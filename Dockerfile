# Build stage
FROM golang:1.23-alpine AS builder

# Install git and ca-certificates (needed for go mod download)
RUN apk add --no-cache git ca-certificates tzdata

# Set working directory
WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build arguments for version info
ARG VERSION=dev
ARG GIT_COMMIT=unknown
ARG BUILD_DATE=unknown

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags "-X github.com/certwatch-app/cw-agent/internal/version.Version=${VERSION} \
              -X github.com/certwatch-app/cw-agent/internal/version.GitCommit=${GIT_COMMIT} \
              -X github.com/certwatch-app/cw-agent/internal/version.BuildDate=${BUILD_DATE} \
              -s -w" \
    -o /cw-agent ./cmd/cw-agent

# Final stage - using distroless for minimal attack surface
FROM gcr.io/distroless/static-debian12:nonroot

# Copy timezone data
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo

# Copy CA certificates
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Copy binary
COPY --from=builder /cw-agent /cw-agent

# Set user
USER nonroot:nonroot

# Set entrypoint
ENTRYPOINT ["/cw-agent"]

# Default command
CMD ["start", "-c", "/etc/certwatch/certwatch.yaml"]

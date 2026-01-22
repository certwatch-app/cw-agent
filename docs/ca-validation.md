# CA Validation Guide

This guide explains how to configure Certificate Authority (CA) validation in the CertWatch Agent for monitoring certificates signed by custom or internal CAs.

## Table of Contents

- [Overview](#overview)
- [Configuration Options](#configuration-options)
- [Trust Modes](#trust-modes)
- [Validation Modes](#validation-modes)
- [CA Bundle Sources](#ca-bundle-sources)
- [Per-Certificate Overrides](#per-certificate-overrides)
- [Examples](#examples)
- [Metrics](#metrics)
- [Troubleshooting](#troubleshooting)
- [Security Considerations](#security-considerations)

## Overview

By default, the CertWatch Agent validates certificates against the operating system's trusted CA store. For internal services or private infrastructure, you can configure custom CA validation to:

- **Validate certificates signed by internal CAs**: Monitor private services without false alerts
- **Combine system and custom CAs**: Support both public and internal certificates
- **Disable validation selectively**: Handle self-signed certificates in development environments
- **Override per certificate**: Apply different CA settings to individual certificates

## Configuration Options

CA validation is configured in the `ca:` section of `certwatch.yaml`:

```yaml
ca:
  trust_mode: "system"       # Which CAs to trust
  validation_mode: "chain"   # How strictly to validate
  ca_bundles:                # Paths to CA certificate files
    - "/path/to/ca.pem"
  inline_certs:              # Inline PEM certificates
    - |
      -----BEGIN CERTIFICATE-----
      ...
      -----END CERTIFICATE-----
```

### Global vs Per-Certificate Configuration

- **Global configuration**: Applies to all certificates unless overridden
- **Per-certificate configuration**: Overrides global settings for specific certificates

```yaml
# Global CA config
ca:
  trust_mode: "system"
  validation_mode: "chain"

certificates:
  # Uses global config
  - hostname: "www.example.com"
    port: 443

  # Overrides global config
  - hostname: "internal.corp"
    port: 443
    ca:
      trust_mode: "custom"
      ca_bundles:
        - "/etc/ssl/internal-ca.pem"
```

## Trust Modes

Trust mode determines which Certificate Authorities the agent trusts for validation.

### `system` (default)

Uses the operating system's trusted CA certificate store.

```yaml
ca:
  trust_mode: "system"
  validation_mode: "chain"
```

**Use when:**
- Monitoring public websites
- Certificates are signed by well-known CAs (Let's Encrypt, DigiCert, etc.)
- No custom CAs are needed

**Supported platforms:**
- Linux: `/etc/ssl/certs/`
- macOS: System Keychain
- Windows: Certificate Store

### `custom`

Uses only custom CA certificates specified in configuration.

```yaml
ca:
  trust_mode: "custom"
  validation_mode: "chain"
  ca_bundles:
    - "/etc/ssl/certs/internal-root-ca.pem"
    - "/etc/ssl/certs/internal-intermediate-ca.pem"
```

**Use when:**
- All monitored services use internal CAs
- Isolating trust to specific CAs only
- Running in air-gapped environments

**Requirements:**
- Must specify at least one CA via `ca_bundles` or `inline_certs`
- CA certificates must be in PEM format

### `combined`

Trusts both system CAs and custom CAs.

```yaml
ca:
  trust_mode: "combined"
  validation_mode: "chain"
  ca_bundles:
    - "/etc/ssl/certs/internal-ca.pem"
```

**Use when:**
- Monitoring both public and internal services
- Need to support certificates from multiple CA hierarchies
- Mixed environment with external and internal CAs

## Validation Modes

Validation mode controls how strictly certificates are validated.

### `chain` (default)

Full certificate chain validation using Go's `x509.Verify()`.

```yaml
ca:
  validation_mode: "chain"
```

**Validates:**
- ✅ Certificate signature
- ✅ Chain to trusted root
- ✅ Certificate expiration
- ✅ Hostname match (SNI)
- ✅ Not-before date
- ✅ Key usage constraints

**Use for:**
- Production environments
- Maximum security validation
- Compliance requirements

### `basic`

Basic validation without hostname verification.

```yaml
ca:
  validation_mode: "basic"
```

**Validates:**
- ✅ Certificate signature
- ✅ Chain to trusted root
- ✅ Certificate expiration
- ❌ Hostname match (skipped)

**Use for:**
- Wildcard certificates
- SAN certificates with complex naming
- IP-based certificates

### `none`

Skips CA validation entirely (only checks expiration and basic issues).

```yaml
ca:
  validation_mode: "none"
```

**Validates:**
- ✅ Certificate expiration
- ✅ Weak signature algorithms
- ❌ CA trust chain (skipped)
- ❌ Hostname match (skipped)

**Use for:**
- Self-signed certificates
- Development environments
- Testing and debugging

⚠️ **Warning**: Use `none` mode sparingly. It disables important security checks.

## CA Bundle Sources

CA certificates can be loaded from two sources:

### File Paths (`ca_bundles`)

Load CA certificates from PEM files on disk.

```yaml
ca:
  ca_bundles:
    - "/etc/ssl/certs/root-ca.pem"
    - "/etc/ssl/certs/intermediate-ca.pem"
```

**File requirements:**
- Must be in PEM format
- Can contain multiple certificates
- Must be readable by the agent
- Must not be world-writable (security check)
- Must not be symlinks (security check)

**Example PEM format:**
```pem
-----BEGIN CERTIFICATE-----
MIIDXTCCAkWgAwIBAgIJAKL0UG+mRKUzMA0GCSqGSIb3DQEBCwUAMEUxCzAJBgNV
...
-----END CERTIFICATE-----
-----BEGIN CERTIFICATE-----
MIIDYTCCAkmgAwIBAgIJAKL0UG+mRKU0MA0GCSqGSIb3DQEBCwUAMEUxCzAJBgNV
...
-----END CERTIFICATE-----
```

### Inline Certificates (`inline_certs`)

Embed CA certificates directly in the configuration file.

```yaml
ca:
  inline_certs:
    - |
      -----BEGIN CERTIFICATE-----
      MIIDXTCCAkWgAwIBAgIJAKL0UG+mRKUzMA0GCSqGSIb3DQEBCwUAMEUxCzAJBgNV
      ...
      -----END CERTIFICATE-----
```

**Use cases:**
- Kubernetes ConfigMaps
- Environment variables
- Automated deployments
- Version control tracking

## Per-Certificate Overrides

Override global CA settings for individual certificates.

```yaml
# Global default
ca:
  trust_mode: "system"
  validation_mode: "chain"

certificates:
  # Public service - uses global system CAs
  - hostname: "www.example.com"
    port: 443

  # Internal service - overrides with custom CA
  - hostname: "internal-api.corp"
    port: 443
    ca:
      trust_mode: "custom"
      validation_mode: "chain"
      ca_bundles:
        - "/etc/ssl/internal-ca.pem"

  # Self-signed dev server - disables validation
  - hostname: "dev.local"
    port: 8443
    ca:
      validation_mode: "none"
```

## Examples

See the `examples/` directory for complete configuration examples:

- **[ca-system-mode.yaml](../examples/ca-system-mode.yaml)** - System CA validation
- **[ca-custom-internal.yaml](../examples/ca-custom-internal.yaml)** - Custom internal CA
- **[ca-combined-mode.yaml](../examples/ca-combined-mode.yaml)** - System + custom CAs
- **[ca-inline-certs.yaml](../examples/ca-inline-certs.yaml)** - Inline PEM certificates
- **[ca-per-cert-override.yaml](../examples/ca-per-cert-override.yaml)** - Per-certificate overrides
- **[ca-validation-modes.yaml](../examples/ca-validation-modes.yaml)** - Different validation modes

## Metrics

The agent exposes Prometheus metrics for CA validation:

### `certwatch_certificate_ca_validation`

CA validation status for each certificate.

**Labels:**
- `hostname`: Certificate hostname
- `port`: Certificate port
- `validation_mode`: Validation mode used (`none`, `basic`, `chain`)

**Values:**
- `1` = Validation succeeded
- `0` = Validation failed

**Example:**
```prometheus
certwatch_certificate_ca_validation{hostname="internal.corp",port="443",validation_mode="chain"} 1
```

### `certwatch_certificate_trusted_root_info`

Trusted root CA information.

**Labels:**
- `hostname`: Certificate hostname
- `port`: Certificate port
- `root_cn`: Common Name of trusted root CA

**Values:**
- `1` = Always 1 (info metric)

**Example:**
```prometheus
certwatch_certificate_trusted_root_info{hostname="internal.corp",port="443",root_cn="Internal Root CA"} 1
```

## Troubleshooting

### "Certificate is not trusted"

**Error**: `ca_validation_failed: certificate validation failed: x509: certificate signed by unknown authority`

**Solutions:**
1. Verify CA bundle path is correct
2. Check CA bundle contains root and intermediate CAs
3. Ensure file permissions are readable
4. Test with `openssl verify -CAfile /path/to/ca.pem /path/to/cert.pem`

### "CA bundle file not found"

**Error**: `ca bundle file not found: /path/to/ca.pem`

**Solutions:**
1. Check file path is absolute (not relative)
2. Verify file exists: `ls -l /path/to/ca.pem`
3. Check agent has read permission
4. Use inline_certs as alternative

### "CA bundle has insecure permissions"

**Error**: `ca bundle has insecure permissions: /path/to/ca.pem (permissions: 0666)`

**Solutions:**
```bash
# Fix permissions (remove world-write)
chmod 644 /path/to/ca.pem

# Verify permissions
ls -l /path/to/ca.pem
# Should show: -rw-r--r--
```

### "No valid certificates found in bundle"

**Error**: `ca bundle contains no valid certificates`

**Solutions:**
1. Verify PEM format: file should contain `-----BEGIN CERTIFICATE-----`
2. Check for parsing errors: `openssl x509 -in /path/to/ca.pem -text -noout`
3. Ensure certificates (not private keys) are in the bundle
4. Try loading in separate files if bundle is corrupted

### Validation fails for valid certificate

**Troubleshooting steps:**
1. Check trust_mode matches CA source
2. Verify complete certificate chain is available
3. Test with `validation_mode: "basic"` to isolate hostname issues
4. Use `openssl s_client -connect hostname:port -CAfile /path/to/ca.pem`

## Security Considerations

### File Permissions

CA bundle files are validated for secure permissions:

- **Rejected**: World-writable files (`0666`, `0777`)
- **Warned**: Group-writable files (`0664`)
- **Accepted**: Owner-only or read-only (`0600`, `0644`)

```bash
# Recommended permissions
chmod 600 /etc/ssl/certs/internal-ca.pem  # Owner read-only
chmod 644 /etc/ssl/certs/internal-ca.pem  # World-readable
```

### TOCTOU Protection

The agent validates files before loading to prevent Time-of-Check-Time-of-Use attacks:

- Files are stat'd and validated before reading
- Symlinks are rejected (use real files)
- Directories are rejected (use files)

### CA Bundle Integrity

**Best practices:**
- Store CA bundles in read-only locations (`/etc/ssl/certs/`)
- Use configuration management to distribute CA updates
- Verify CA fingerprints after updates
- Rotate CAs using combined mode during transition

### Trust Store Updates

When updating CA certificates:

```yaml
# Phase 1: Add new CA alongside old (combined mode)
ca:
  trust_mode: "combined"
  ca_bundles:
    - "/etc/ssl/certs/old-ca.pem"
    - "/etc/ssl/certs/new-ca.pem"

# Phase 2: After all certs renewed, remove old CA
ca:
  trust_mode: "custom"
  ca_bundles:
    - "/etc/ssl/certs/new-ca.pem"
```

### Kubernetes Secrets

Store CA certificates in Kubernetes Secrets (not ConfigMaps):

```bash
# Create secret from CA bundle
kubectl create secret generic internal-ca \
  --from-file=ca.pem=/path/to/ca.pem

# Mount in agent pod
volumeMounts:
  - name: ca-bundle
    mountPath: /etc/ssl/certs
    readOnly: true
volumes:
  - name: ca-bundle
    secret:
      secretName: internal-ca
      defaultMode: 0400  # Read-only
```

## Advanced Configuration

### Multiple CA Hierarchies

Support certificates from multiple CA hierarchies:

```yaml
ca:
  trust_mode: "custom"
  validation_mode: "chain"
  ca_bundles:
    # CA Hierarchy 1 (Production)
    - "/etc/ssl/certs/prod-root-ca.pem"
    - "/etc/ssl/certs/prod-intermediate-ca.pem"
    # CA Hierarchy 2 (Development)
    - "/etc/ssl/certs/dev-root-ca.pem"
```

### Environment-Specific Configuration

Use environment variables for portability:

```yaml
ca:
  trust_mode: "custom"
  ca_bundles:
    - "${CA_BUNDLE_PATH}/root-ca.pem"
  inline_certs:
    - "${CA_INLINE_CERT}"
```

### Container Environments

Mount CA bundles from host or secrets:

```dockerfile
# Dockerfile
COPY ca-certificates/*.pem /usr/local/share/ca-certificates/
RUN update-ca-certificates
```

```yaml
# certwatch.yaml (use system CAs with custom additions)
ca:
  trust_mode: "system"  # Will include copied certs
  validation_mode: "chain"
```

## Further Reading

- [TLS Certificate Management Best Practices](https://tools.ietf.org/html/rfc5280)
- [X.509 Certificate Path Validation](https://tools.ietf.org/html/rfc5280#section-6)
- [Kubernetes TLS Management](https://kubernetes.io/docs/tasks/tls/)
- [OpenSSL CA Management](https://www.openssl.org/docs/man1.1.1/man1/ca.html)

## Support

If you encounter issues:

1. Check agent logs for detailed error messages
2. Verify configuration with `cw-agent validate --config certwatch.yaml`
3. Test CA validation with OpenSSL tools
4. Open an issue on [GitHub](https://github.com/certwatch-app/cw-agent/issues)

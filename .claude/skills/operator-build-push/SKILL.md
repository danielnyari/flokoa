---
name: operator-build-push
description: Build and push Docker images for Flokoa operator components to ghcr.io. Use when the user asks to build images, push to registry, create releases, or deploy operator containers. Covers operator, server, A2A plugin, and CLI images.
---

# Build and Push Operator Images

All commands must be run from the `operator/` directory.

## Image Registry

All images are pushed to `ghcr.io/danielnyari/`:

| Component | Image | Base Tag |
|-----------|-------|----------|
| Operator | `ghcr.io/danielnyari/flokoa-operator` | `VERSION` (default: 0.0.6) |
| Server | `ghcr.io/danielnyari/flokoa-server` | `VERSION` |
| A2A Plugin | `ghcr.io/danielnyari/flokoa-a2a-plugin` | `VERSION` + `latest` |
| Flokoa CLI | `ghcr.io/danielnyari/flokoa-cli` | `VERSION` + `latest` |

## Before Building

Always ensure code generation is current:

```bash
make manifests generate
```

For gRPC changes:
```bash
make buf-generate
```

## Build Commands

### Build All Core Images (operator + server + A2A plugin)
```bash
make docker-build
```

### Build Individual Images
```bash
make docker-build                  # Operator + server + A2A plugin
make docker-build-plugins          # A2A plugin only
make docker-build-flokoa-cli       # Python SDK CLI image only
```

### Push Commands
```bash
make docker-push                   # Push operator + server + A2A plugin
make docker-push-plugins           # Push A2A plugin (tagged as VERSION + latest)
make docker-push-flokoa-cli        # Push CLI image (tagged as VERSION + latest)
```

## Custom Version or Image Tag

Override the version:
```bash
make docker-build VERSION=0.1.0
make docker-push VERSION=0.1.0
```

Override the full image tag:
```bash
make docker-build IMG=my-registry.io/operator:dev
make docker-push IMG=my-registry.io/operator:dev
```

## Cross-Platform Builds

Build for multiple architectures (requires Docker BuildKit):
```bash
make docker-buildx IMG=ghcr.io/danielnyari/flokoa-operator:0.0.6
```

Supported platforms: `linux/arm64`, `linux/amd64`, `linux/s390x`, `linux/ppc64le`.

## Build Flow

1. `make docker-build` runs:
   - `docker build -t ghcr.io/danielnyari/flokoa-operator:0.0.6 .` (operator)
   - `docker build -f server/Dockerfile -t ghcr.io/danielnyari/flokoa-server:0.0.6 .` (server)
   - `docker build -f plugins/a2a/Dockerfile -t ghcr.io/danielnyari/flokoa-a2a-plugin:0.0.6 .` (plugin)

2. `make docker-push` pushes all three and also tags the A2A plugin as `latest`.

3. The Flokoa CLI image is built separately from `sdk/python/Dockerfile.managed`.

## Container Tool

Default is `docker`. To use podman:
```bash
make docker-build CONTAINER_TOOL=podman
```

## OLM Bundle (Operator Lifecycle Manager)

For OLM-based distribution:
```bash
make bundle          # Generate bundle manifests
make bundle-build    # Build bundle image
make bundle-push     # Push bundle image
make catalog-build   # Build catalog image
make catalog-push    # Push catalog image
```

## Installer YAML

Generate a single consolidated YAML for direct deployment:
```bash
make build-installer
```

Output: `dist/install.yaml` containing CRDs + deployment manifests.

## Authentication

Ensure you're authenticated to ghcr.io before pushing:
```bash
echo $GITHUB_TOKEN | docker login ghcr.io -u USERNAME --password-stdin
```

## Dockerfile Details

The operator uses a multi-stage build:
- **Builder**: `golang:1.24` - Compiles the manager binary
- **Runtime**: `gcr.io/distroless/static:nonroot` - Minimal attack surface
- Runs as non-root user (UID/GID 65532)

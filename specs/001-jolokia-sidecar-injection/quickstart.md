# Quickstart: Jolokia Sidecar Injection Webhook

**Feature**: 001-jolokia-sidecar-injection
**Date**: 2026-02-22

## Prerequisites

- Go 1.25.3+
- Docker 17.03+
- kubectl v1.29+
- [Kind](https://kind.sigs.k8s.io/) (for local testing)
- Access to a Kubernetes 1.33+ cluster (or Kind cluster)

## Local Development

### 1. Run unit tests

```bash
make test
```

Tests use `envtest` (real K8s API server + etcd, no full cluster needed). Covers:
- Annotation parsing and validation (`internal/jolokia/`)
- Webhook injection logic (`internal/webhook/v1/`)

### 2. Run locally against a cluster

```bash
# Uses your current kubeconfig context
make run
```

The webhook registers with the API server. Create a test Pod:

```bash
kubectl apply -f config/samples/pod-with-jolokia.yaml
```

### 3. Run linter

```bash
make lint       # Check only
make lint-fix   # Auto-fix
```

## Build & Deploy

### 1. Build the operator image

```bash
export IMG=ghcr.io/alxgomz/jolokia-operator:dev
make docker-build IMG=$IMG
```

### 2. Deploy to Kind (local)

```bash
kind create cluster --name jolokia-test
kind load docker-image $IMG --name jolokia-test
make deploy IMG=$IMG
```

### 3. Deploy to a remote cluster

```bash
make docker-push IMG=$IMG
make deploy IMG=$IMG
```

### 4. Configure the default sidecar image (optional)

Edit the manager Deployment to pass the `--sidecar-image` flag:

```bash
kubectl -n jolokia-operator-system edit deployment jolokia-operator-controller-manager
```

Add to the manager container args:

```yaml
args:
  - --sidecar-image=ghcr.io/alxgomz/jolokia-agent:2
```

Or set it via kustomize patch before deploying.

## Testing Sidecar Injection

### Annotate a Pod for injection

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: my-java-app
  annotations:
    jolokia.horoa.net/port: "8778"
    jolokia.horoa.net/host: "0.0.0.0"
spec:
  containers:
    - name: app
      image: openjdk:21-slim
      command: ["java", "-jar", "/app.jar"]
```

After creation, verify injection:

```bash
kubectl get pod my-java-app -o jsonpath='{.spec.initContainers[?(@.name=="jolokia-agent")]}'
```

### Verify the sidecar configuration

```bash
# Check sidecar args
kubectl get pod my-java-app -o jsonpath='{.spec.initContainers[?(@.name=="jolokia-agent")].args}'
# Expected: ["port=8778","host=0.0.0.0"]

# Check shared PID namespace
kubectl get pod my-java-app -o jsonpath='{.spec.shareProcessNamespace}'
# Expected: true

# Check volume
kubectl get pod my-java-app -o jsonpath='{.spec.volumes[?(@.name=="jolokia-tmp")]}'
# Expected: {"emptyDir":{},"name":"jolokia-tmp"}
```

### Test with resource limits

```yaml
annotations:
  jolokia.horoa.net/port: "8778"
  jolokia.horoa.net/rsc-limits-cpu: "200m"
  jolokia.horoa.net/rsc-limits-memory: "128Mi"
  jolokia.horoa.net/rsc-requests-cpu: "50m"
  jolokia.horoa.net/rsc-requests-memory: "64Mi"
```

### Test with custom mount path and target process

```yaml
annotations:
  jolokia.horoa.net/port: "8778"
  jolokia.horoa.net/mnt: "/opt/jolokia"
  jolokia.horoa.net/target-process: "java"
```

### Test validation rejection

```yaml
annotations:
  jolokia.horoa.net/port: "8778"
  jolokia.horoa.net/invalidKey: "value"
```

Expected: Pod creation is rejected with an error listing `invalidKey` as invalid.

## E2E Tests

```bash
# Requires a dedicated Kind cluster
make test-e2e
```

## Regenerate After Changes

After editing `*_types.go` or kubebuilder markers:

```bash
make manifests generate
```

## Key Files

| File | Purpose |
|---|---|
| `internal/jolokia/annotations.go` | Annotation parsing, key validation |
| `internal/jolokia/injector.go` | Pod mutation logic (sidecar builder) |
| `internal/webhook/v1/pod_webhook.go` | Webhook handlers (defaulter + validator) |
| `cmd/main.go` | `--sidecar-image` flag, webhook registration |
| `config/webhook/manifests.yaml` | Generated webhook configuration (DO NOT EDIT) |

## Annotation Reference

### Jolokia Agent Options (58 valid keys)

See [spec.md Appendix A](./spec.md#appendix-a--valid-jolokia-jvm-agent-option-names) for the full list.

### Operator-Specific Annotations

| Key | Purpose | Default |
|---|---|---|
| `jolokia.horoa.net/mnt` | Volume mount path | `/tmp` |
| `jolokia.horoa.net/rsc-limits-cpu` | Sidecar CPU limit | *(none)* |
| `jolokia.horoa.net/rsc-limits-memory` | Sidecar memory limit | *(none)* |
| `jolokia.horoa.net/rsc-requests-cpu` | Sidecar CPU request | *(none)* |
| `jolokia.horoa.net/rsc-requests-memory` | Sidecar memory request | *(none)* |
| `jolokia.horoa.net/target-process` | Target process name | *(auto-detect)* |
| `jolokia.horoa.net/sidecar-image` | Override sidecar image | Operator default or `ghcr.io/alxgomz/jolokia-agent:2` |

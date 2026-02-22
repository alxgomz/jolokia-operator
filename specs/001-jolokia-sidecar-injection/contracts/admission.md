# Contract: Webhook Admission

**Feature**: 001-jolokia-sidecar-injection
**Date**: 2026-02-22
**Source**: [spec.md](../spec.md), [research.md](../research.md), [data-model.md](../data-model.md)

## Overview

Two admission webhooks intercept Pod CREATE requests:

1. **Mutating webhook** (`PodCustomDefaulter.Default`) — validates annotation keys, then injects the Jolokia sidecar into valid Pods.
2. **Validating webhook** (`PodCustomValidator.ValidateCreate`) — defense-in-depth validation of annotation keys after mutation.

Kubernetes guarantees ordering: all mutating webhooks run before all validating webhooks.

## Mutating Webhook Contract

### Path

`/mutate--v1-pod`

### Trigger

| Field | Value |
|---|---|
| `operations` | `["CREATE"]` |
| `resources` | `["pods"]` |
| `apiGroups` | `[""]` |
| `apiVersions` | `["v1"]` |
| `failurePolicy` | `Fail` |
| `sideEffects` | `None` |
| `reinvocationPolicy` | `IfNeeded` |
| `admissionReviewVersions` | `["v1"]` |
| `namespaceSelector` | Excludes `kube-system`, `kube-public`, `kube-node-lease`, `jolokia-operator-system` |

### Input

A `corev1.Pod` object from the admission request body.

### Decision Logic

```
1. Extract all annotations with prefix "jolokia.horoa.net/"
2. If NO matching annotations → ALLOW (no-op, Pod unchanged)
3. Parse annotation key suffixes:
   a. Classify each as: jolokia-option | operator-specific | invalid
   b. If ANY invalid keys → DENY with error listing all invalid keys
4. If init container named "jolokia-agent" already exists → ALLOW (idempotent skip, Pod unchanged)
5. Mutate Pod:
   a. Resolve sidecar image: annotation > CLI flag > hardcoded default
   b. Build container args from jolokia options (key=value format)
   c. Append native sidecar init container to pod.Spec.InitContainers
   d. Add emptyDir volume "jolokia-tmp" to pod.Spec.Volumes
   e. Set pod.Spec.ShareProcessNamespace = true
6. ALLOW (framework computes JSON patch from mutations)
```

### Output: ALLOW (no annotations)

- **Patch**: empty (no mutations)
- **Warnings**: none

### Output: ALLOW (injection performed)

- **Patch**: JSON patch containing:
  - `add /spec/initContainers/-` — the `jolokia-agent` native sidecar container
  - `add /spec/volumes/-` — the `jolokia-tmp` emptyDir volume
  - `replace /spec/shareProcessNamespace` — set to `true`
- **Warnings**: none

### Output: ALLOW (idempotent skip)

- **Patch**: empty (no mutations)
- **Warnings**: none

### Output: DENY (invalid annotation keys)

- **Allowed**: `false`
- **Status code**: `400` (Bad Request)
- **Status message**: `invalid Jolokia annotation keys: [key1, key2, ...]; valid options are listed at https://jolokia.org/reference/html/manual/agents.html#jvm-agent`

## Injected Sidecar Container Spec

```yaml
name: jolokia-agent
image: <resolved-image>           # annotation > --sidecar-image flag > ghcr.io/alxgomz/jolokia-agent:2
args:                              # from jolokia option annotations (key=value)
  - "port=8778"
  - "host=0.0.0.0"
  - "target-process=org.apache.catalina.startup.Bootstrap"  # or a numeric PID like "42"
restartPolicy: Always              # KEP-753 native sidecar
securityContext:
  readOnlyRootFilesystem: true
  allowPrivilegeEscalation: false
  runAsNonRoot: true
  capabilities:
    drop: ["ALL"]
    add: ["SYS_PTRACE"]
  seccompProfile:
    type: RuntimeDefault
resources:                         # from rsc-* annotations (all optional)
  limits:
    cpu: <rsc-limits-cpu>
    memory: <rsc-limits-memory>
  requests:
    cpu: <rsc-requests-cpu>
    memory: <rsc-requests-memory>
volumeMounts:
  - name: jolokia-tmp
    mountPath: <mnt-annotation or /tmp>
```

## Injected Volume Spec

```yaml
name: jolokia-tmp
emptyDir: {}
```

## Validating Webhook Contract

### Path

`/validate--v1-pod`

### Trigger

Same as mutating webhook (Pod CREATE, same namespace selector, `failurePolicy: Fail`).

### Input

A `corev1.Pod` object (already mutated by the defaulter).

### Decision Logic

```
1. Extract all annotations with prefix "jolokia.horoa.net/"
2. If NO matching annotations → ALLOW
3. Parse annotation key suffixes:
   a. Classify each as: jolokia-option | operator-specific | invalid
   b. If ANY invalid keys → DENY with error listing all invalid keys
4. ALLOW
```

### Output: ALLOW

- **Warnings**: none

### Output: DENY (invalid annotation keys)

- **Allowed**: `false`
- **Status code**: `400` (Bad Request)
- **Status message**: Same format as mutating webhook denial

## Observability Contract

### Structured Log Events

| Event | Level | Fields |
|---|---|---|
| Injection performed | `Info` | `pod`, `namespace`, `options` (list of applied Jolokia args) |
| Injection skipped (no annotations) | Debug (no log) | — |
| Injection skipped (idempotent) | `Info` | `pod`, `namespace`, `reason: "already injected"` |
| Validation rejection | `Info` | `pod`, `namespace`, `invalidKeys` (list) |

### Prometheus Metrics

| Metric | Type | Labels | Description |
|---|---|---|---|
| `jolokia_admission_requests_total` | Counter | `webhook` (`mutating`\|`validating`), `decision` (`allow`\|`deny`\|`skip`) | Total admission requests processed |
| `jolokia_admission_duration_seconds` | Histogram | `webhook` | Admission handler latency |
| `jolokia_injections_total` | Counter | — | Total successful sidecar injections |

## Error Handling

| Scenario | Behavior |
|---|---|
| Pod has no `jolokia.horoa.net/*` annotations | Pass-through (ALLOW, no mutation) |
| Pod has invalid annotation keys | DENY with descriptive error (both webhooks) |
| Pod already has `jolokia-agent` init container | ALLOW without mutation (idempotent) |
| Invalid resource quantity in `rsc-*` annotation | Not validated by webhook (Kubernetes API server validates resource quantities) |
| Webhook handler panics | controller-runtime recovers; returns 500 to API server; `failurePolicy: Fail` blocks Pod creation |
| Webhook TLS certificate expired | API server cannot reach webhook; `failurePolicy: Fail` blocks Pod creation |

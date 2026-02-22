# Data Model: Jolokia Sidecar Injection Webhook

**Feature**: 001-jolokia-sidecar-injection
**Date**: 2026-02-22
**Source**: [spec.md](./spec.md), [research.md](./research.md)

## Entities

### 1. JolokiaAnnotation

Represents a parsed annotation from a Pod's metadata that uses the `jolokia.horoa.net/` prefix.

```go
package jolokia

// AnnotationPrefix is the prefix for all Jolokia-related annotations.
const AnnotationPrefix = "jolokia.horoa.net/"

// ParsedAnnotations holds the categorized annotations from a Pod.
type ParsedAnnotations struct {
    // JolokiaOptions maps Jolokia agent option names to their values.
    // These are passed as container args to the sidecar.
    // Example: {"port": "9090", "host": "0.0.0.0"}
    JolokiaOptions map[string]string

    // MountPath is the mount path for the jolokia-tmp volume.
    // Defaults to "/tmp" if the mnt annotation is absent.
    MountPath string

    // SidecarImage overrides the default sidecar container image.
    // Empty string means "use operator default".
    SidecarImage string

    // TargetProcess is the PID or Java main class name of the process to attach to.
    // Must be either a numeric PID (e.g., "1", "42") or a fully qualified Java
    // class name as returned by the Jolokia agent `list` command
    // (e.g., "org.apache.catalina.startup.Bootstrap").
    // Empty string means "use sidecar's default discovery".
    TargetProcess string

    // Resources holds the sidecar resource requests/limits.
    Resources SidecarResources

    // InvalidKeys lists annotation keys that are neither valid
    // Jolokia options nor recognized operator-specific keys.
    InvalidKeys []string
}

// SidecarResources holds optional resource allocation overrides.
type SidecarResources struct {
    LimitsCPU      string // from rsc-limits-cpu
    LimitsMemory   string // from rsc-limits-memory
    RequestsCPU    string // from rsc-requests-cpu
    RequestsMemory string // from rsc-requests-memory
}
```

**Validation rules**:
- Annotation key suffix (after prefix) MUST match one of:
  - 58 valid Jolokia option names (see Appendix A in spec)
  - 7 operator-specific keys: `mnt`, `rsc-limits-cpu`, `rsc-limits-memory`, `rsc-requests-cpu`, `rsc-requests-memory`, `target-process`, `sidecar-image`
- Values are NOT validated (opaque strings, per FR-007)
- Empty values are valid (per edge case in spec)

### 2. ValidJolokiaOptions

The set of 58 valid Jolokia JVM agent option names. Implemented as a `map[string]struct{}` for O(1) lookup.

```go
// validJolokiaOptions is the exhaustive set of valid Jolokia 2.x
// JVM agent option names (Table 5 + Table 1 from official docs).
var validJolokiaOptions = map[string]struct{}{
    "agentContext":          {},
    "agentDescription":      {},
    "agentId":               {},
    // ... all 58 options (see Appendix A in spec)
    "user":                  {},
    "useRestrictorService":  {},
}

// operatorAnnotations is the set of operator-specific annotation
// key suffixes that are valid but NOT Jolokia options.
var operatorAnnotations = map[string]struct{}{
    "mnt":                {},
    "rsc-limits-cpu":     {},
    "rsc-limits-memory":  {},
    "rsc-requests-cpu":   {},
    "rsc-requests-memory":{},
    "target-process":     {},
    "sidecar-image":      {},
}
```

### 3. InjectionResult

Represents the outcome of the sidecar injection logic.

```go
// InjectionResult describes what the injector did to a Pod.
type InjectionResult struct {
    // Injected is true if the sidecar was added.
    Injected bool

    // Skipped is true if injection was skipped (no annotations or already injected).
    Skipped bool

    // SkipReason explains why injection was skipped.
    SkipReason string

    // OptionsApplied lists the Jolokia options passed to the sidecar.
    OptionsApplied []string
}
```

### 4. PodCustomDefaulter (Webhook Struct)

The webhook handler that performs Pod mutation. Lives in `internal/webhook/v1/`.

```go
// PodCustomDefaulter is responsible for injecting the Jolokia
// sidecar into annotated Pods.
type PodCustomDefaulter struct {
    // DefaultImage is the operator-level default sidecar image,
    // set via --sidecar-image CLI flag.
    DefaultImage string
}
```

**State transitions**: None. The webhook is stateless — each admission request is processed independently.

### 5. PodCustomValidator (Webhook Struct)

The webhook handler that validates Jolokia annotation keys.

```go
// PodCustomValidator validates that Pod annotations use
// recognized Jolokia option names.
type PodCustomValidator struct{}
```

## Relationships

```
Pod (K8s core type)
 │
 ├─ metadata.annotations[jolokia.horoa.net/*]
 │   └─ Parsed into → ParsedAnnotations
 │
 ├─ spec.initContainers
 │   └─ Injected → jolokia-agent (native sidecar)
 │
 ├─ spec.volumes
 │   └─ Injected → jolokia-tmp (emptyDir)
 │
 └─ spec.shareProcessNamespace
     └─ Set to true

PodCustomDefaulter
 │
 ├─ Uses → ParsedAnnotations (annotation parsing)
 ├─ Uses → ValidJolokiaOptions (key validation)
 ├─ Produces → InjectionResult (logging/metrics)
 └─ Reads → DefaultImage (struct field)

PodCustomValidator
 │
 ├─ Uses → ParsedAnnotations (annotation parsing)
 └─ Uses → ValidJolokiaOptions (key validation)
```

## Data Flow

```
Pod CREATE request
 │
 ▼
MutatingWebhook (PodCustomDefaulter.Default)
 │
 ├─ 1. Extract jolokia.horoa.net/* annotations
 ├─ 2. Parse into ParsedAnnotations
 ├─ 3. If no jolokia annotations → return (no-op)
 ├─ 4. If InvalidKeys non-empty → return error
 ├─ 5. If already injected (jolokia-agent init container exists) → return (idempotent skip)
 ├─ 6. Resolve sidecar image: annotation > DefaultImage > hardcoded default
 ├─ 7. Build Jolokia args from JolokiaOptions map
 ├─ 8. Build sidecar Container spec
 ├─ 9. Append to pod.Spec.InitContainers
 ├─ 10. Add jolokia-tmp emptyDir volume
 ├─ 11. Set pod.Spec.ShareProcessNamespace = true
 └─ 12. Return (framework computes JSON patch)
 │
 ▼
ValidatingWebhook (PodCustomValidator.ValidateCreate)
 │
 ├─ 1. Extract jolokia.horoa.net/* annotations
 ├─ 2. Parse into ParsedAnnotations
 ├─ 3. If no jolokia annotations → return allowed
 ├─ 4. If InvalidKeys non-empty → return denied (list invalid keys)
 └─ 5. Return allowed
```
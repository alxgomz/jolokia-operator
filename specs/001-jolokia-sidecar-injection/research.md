# Research: Jolokia Sidecar Injection Webhook

**Feature**: 001-jolokia-sidecar-injection
**Date**: 2026-02-22
**Purpose**: Resolve technical unknowns identified in the Technical Context before Phase 1 design.

## R-001: CustomDefaulter Interface for Pod Mutation

**Question**: How does the controller-runtime `CustomDefaulter` interface work for mutating core types (Pods)?

**Decision**: Use the scaffolded `PodCustomDefaulter` struct with a `Default(ctx context.Context, obj *corev1.Pod) error` method. The method receives the Pod as a typed pointer — mutations are applied in-place. The webhook framework handles JSON patch generation automatically by diffing the object before and after `Default()` returns.

**Rationale**: The Kubebuilder scaffold already generated the correct interface implementation in `internal/webhook/v1/pod_webhook.go`. The `Default` method signature accepts `*corev1.Pod` (typed, not `runtime.Object`) because the scaffold uses the typed `CustomDefaulter` interface introduced in controller-runtime v0.15+. The framework wraps this into an admission handler that:
1. Decodes the admission request into a `corev1.Pod`
2. Calls `Default(ctx, pod)`
3. Marshals the mutated Pod
4. Computes a JSON patch between original and mutated
5. Returns the admission response with the patch

**Alternatives considered**:
- Raw `admission.Handler` interface: More flexible but requires manual decoding, patch generation, and error handling. Unnecessary complexity.
- `Mutating` webhook via `admission.WithCustomDefaulter()`: This is what `ctrl.NewWebhookManagedBy().WithDefaulter()` does under the hood — same thing.

## R-002: CustomValidator Interface for Annotation Validation

**Question**: Should annotation validation happen in the defaulter (mutating webhook) or validator (validating webhook)?

**Decision**: Validation happens in the `PodCustomValidator.ValidateCreate()` method (validating webhook). Injection happens in `PodCustomDefaulter.Default()` (mutating webhook). The mutating webhook runs first (Kubernetes ordering: mutating → validating), so the validating webhook sees the already-mutated Pod.

**Rationale**: Separation of concerns. The mutating webhook's job is to inject — it should succeed if the Pod has valid annotations. The validating webhook's job is to reject — it checks that annotation keys are valid. This follows the Kubernetes convention where mutating webhooks transform and validating webhooks enforce.

**Important nuance**: Both webhooks fire on the same Pod CREATE. The mutating webhook (defaulter) must also validate keys before injecting, because it should NOT inject a sidecar into a Pod with invalid annotations. The flow is:
1. Mutating webhook: Parse annotations → if any `jolokia.horoa.net/*` present, validate keys → if valid, inject sidecar → if invalid, return error (this blocks creation)
2. Validating webhook: Parse annotations → validate keys → if invalid, reject with descriptive error

Since the mutating webhook already rejects invalid keys, the validating webhook acts as a defense-in-depth layer. However, per the spec's `failurePolicy: Fail`, if the mutating webhook is unavailable, Pods are blocked entirely — so the validating webhook primarily catches edge cases where another mutating webhook modifies annotations after ours runs.

**Alternatives considered**:
- Validation only in mutating webhook: Simpler but loses defense-in-depth. If `reinvocationPolicy: IfNeeded` triggers and another webhook adds invalid annotations, we'd miss them.
- Validation only in validating webhook, injection in mutating webhook (no validation): The mutating webhook would inject blindly, then the validating webhook would reject. This wastes mutation work and the rejection message would be confusing ("invalid annotation" on a Pod that was already mutated).

## R-003: Native Sidecar Injection Pattern (KEP-753)

**Question**: What is the exact container spec for injecting a native sidecar init container?

**Decision**: Inject the sidecar as an entry in `pod.Spec.InitContainers` with `RestartPolicy` set to `corev1.ContainerRestartPolicyAlways`. This is the KEP-753 native sidecar pattern, GA since Kubernetes 1.29.

**Rationale**: The Go representation is:
```go
corev1.Container{
    Name:          "jolokia-agent",
    Image:         resolvedImage, // from annotation > CLI flag > default
    Args:          jolokiaArgs,   // ["port=8778", "host=0.0.0.0", ...]
    RestartPolicy: ptr.To(corev1.ContainerRestartPolicyAlways),
    SecurityContext: &corev1.SecurityContext{
        ReadOnlyRootFilesystem:   ptr.To(true),
        AllowPrivilegeEscalation: ptr.To(false),
        RunAsNonRoot:             ptr.To(true),
        Capabilities: &corev1.Capabilities{
            Drop: []corev1.Capability{"ALL"},
            Add:  []corev1.Capability{"SYS_PTRACE"},
        },
        SeccompProfile: &corev1.SeccompProfile{
            Type: corev1.SeccompProfileTypeRuntimeDefault,
        },
    },
    VolumeMounts: []corev1.VolumeMount{
        {
            Name:      "jolokia-tmp",
            MountPath: mountPath, // from annotation or "/tmp"
        },
    },
}
```

The sidecar is appended to `pod.Spec.InitContainers` (not `Containers`). Kubernetes 1.29+ recognizes `RestartPolicy: Always` on init containers as native sidecars — they start before regular containers, keep running alongside them, and are terminated after regular containers exit.

**Alternatives considered**:
- Regular container (non-init): Forbidden by Constitution Principle V. Legacy sidecar pattern.
- Init container without `RestartPolicy: Always`: Would run once and exit before the main container starts — useless for a persistent agent.

## R-004: CLI Flag Wiring Pattern

**Question**: How to wire `--sidecar-image` from `cmd/main.go` through to the webhook handler?

**Decision**: Add `flag.StringVar(&sidecarImage, "sidecar-image", "ghcr.io/alxgomz/jolokia-agent:2", ...)` in `cmd/main.go` before `flag.Parse()`. Pass the value to `PodCustomDefaulter` as a struct field. Modify `SetupPodWebhookWithManager` to accept the image string (or an options struct).

**Rationale**: The existing `cmd/main.go` already uses Go's `flag` package (not cobra directly) for all CLI flags. The pattern is consistent:
```go
// In cmd/main.go, before flag.Parse():
var sidecarImage string
flag.StringVar(&sidecarImage, "sidecar-image", "ghcr.io/alxgomz/jolokia-agent:2",
    "Default container image for the Jolokia agent sidecar.")

// After flag.Parse(), when setting up webhooks:
if os.Getenv("ENABLE_WEBHOOKS") != "false" {
    if err := webhookv1.SetupPodWebhookWithManager(mgr, sidecarImage); err != nil {
        ...
    }
}
```

The `SetupPodWebhookWithManager` function signature changes from `(mgr ctrl.Manager) error` to `(mgr ctrl.Manager, defaultImage string) error`, and passes the image to the `PodCustomDefaulter` struct:
```go
func SetupPodWebhookWithManager(mgr ctrl.Manager, defaultImage string) error {
    return ctrl.NewWebhookManagedBy(mgr, &corev1.Pod{}).
        WithValidator(&PodCustomValidator{}).
        WithDefaulter(&PodCustomDefaulter{DefaultImage: defaultImage}).
        Complete()
}
```

**Alternatives considered**:
- Environment variable (`JOLOKIA_SIDECAR_IMAGE`): Less explicit, harder to discover, inconsistent with existing flag-based configuration pattern.
- ConfigMap-based configuration: Over-engineered for a single string value. Would require watching the ConfigMap and add latency/complexity.
- Struct with multiple fields (options pattern): Premature — only one configurable field exists. Can refactor later if more options are added.

## R-005: Webhook Marker Configuration

**Question**: How to configure `reinvocationPolicy: IfNeeded` and restrict to CREATE-only via kubebuilder markers?

**Decision**: Update the `+kubebuilder:webhook` marker on the defaulter. The current marker includes `verbs=create;update` — change to `verbs=create` (spec says Pod CREATE only). For `reinvocationPolicy`, this is NOT supported as a kubebuilder marker — it must be set via a kustomize patch on the generated `manifests.yaml`.

**Rationale**: The current marker is:
```go
// +kubebuilder:webhook:path=/mutate--v1-pod,mutating=true,failurePolicy=fail,sideEffects=None,groups="",resources=pods,verbs=create;update,versions=v1,name=mpod-v1.kb.io,admissionReviewVersions=v1
```

Changes needed:
1. Change `verbs=create;update` to `verbs=create` (FR-001: only Pod CREATE)
2. Add `reinvocationPolicy=ifNeeded` — kubebuilder markers support this via `reinvocationPolicy` field
3. Similarly update the validating webhook marker to `verbs=create` only
4. `namespaceSelector` for excluding system namespaces — this is NOT a marker field. Must be added via kustomize patch in `config/webhook/` or a post-generation script.

Updated marker:
```go
// +kubebuilder:webhook:path=/mutate--v1-pod,mutating=true,failurePolicy=fail,sideEffects=None,groups="",resources=pods,verbs=create,versions=v1,name=mpod-v1.kb.io,admissionReviewVersions=v1,reinvocationPolicy=ifNeeded
```

For `namespaceSelector`, create a kustomize patch at `config/webhook/namespace_selector_patch.yaml`:
```yaml
- op: add
  path: /webhooks/0/namespaceSelector
  value:
    matchExpressions:
      - key: kubernetes.io/metadata.name
        operator: NotIn
        values:
          - kube-system
          - kube-public
          - kube-node-lease
          - jolokia-operator-system
```

**Alternatives considered**:
- Hard-coding namespaceSelector in the webhook handler (skip Pods from certain namespaces): Violates K8s conventions — namespace filtering should be at the webhook configuration level, not in the handler. The API server skips sending requests entirely for non-matching namespaces, which is more efficient.

## R-006: Jolokia JVM Agent Attach Mechanism

**Question**: How does the Jolokia JVM agent attach to a target process in the shared PID namespace?

**Decision**: The Jolokia JVM agent uses the Java Attach API (`com.sun.tools.attach.VirtualMachine.attach(pid)`) to attach to a running JVM process. In the sidecar container, it discovers the target JVM by scanning `/proc` for Java processes (or uses the `target-process` annotation to filter). The shared PID namespace (`shareProcessNamespace: true`) makes the main container's processes visible in the sidecar's `/proc`. The `SYS_PTRACE` capability is required because the Attach API uses `ptrace` under the hood.

**Rationale**: The Jolokia JVM agent (`jolokia-agent-jvm-2.x.x.jar`) supports two modes:
1. **Premixed** (`-javaagent`): Loaded at JVM startup. Not applicable for sidecar injection (we can't modify the main container's JVM args).
2. **Dynamic attach**: Attaches to a running JVM process via the Attach API. This is the sidecar mode.

Requirements for dynamic attach across containers:
- `shareProcessNamespace: true`: The sidecar must see the main container's PID 1 and its child processes in `/proc`.
- `SYS_PTRACE` capability: The Java Attach API signal mechanism requires ptrace permissions to communicate with the target JVM.
- Shared `/tmp` (or configurable mount): The Attach API uses Unix domain sockets in `/tmp` (specifically `.java_pid<N>` files) for IPC. The `jolokia-tmp` emptyDir volume mounted in both containers ensures this IPC channel works across the container boundary.

The `target-process` annotation maps to a process name filter — the sidecar scans `/proc/*/cmdline` for a process matching the given name and attaches to its PID.

**Alternatives considered**:
- `jcmd` for attach: Also uses the Attach API under the hood — same requirements. The Jolokia agent JAR includes its own attach logic.
- Volume mount at `/proc`: Not needed — `shareProcessNamespace: true` already shares the PID namespace, making `/proc` entries visible across containers.

## R-007: Annotation Parsing and Args Format

**Question**: What exact format should Jolokia options use when passed as container args?

**Decision**: Each Jolokia annotation maps to a container arg in `key=value` format. For example, `jolokia.horoa.net/port: "9090"` becomes the arg `port=9090`. Operator-specific annotations (`mnt`, `rsc-*`, `target-process`, `sidecar-image`) are NOT passed as args — they control operator behavior only.

**Rationale**: The spec (FR-013, clarification Q3) states: "via container command args (e.g., `args: ["port=9090", "host=0.0.0.0"]`)". The Jolokia JVM agent accepts options as `key=value` pairs. The `target-process` annotation is handled separately — it's passed as a distinct arg to the sidecar's entrypoint (not as a Jolokia option), as the sidecar image's entrypoint is expected to parse it for process targeting.

**Alternatives considered**:
- `--key=value` format (with dashes): Depends on the sidecar image's entrypoint. The spec examples use `key=value` without dashes. We follow the spec.
- Comma-separated single arg (`port=9090,host=0.0.0.0`): Less clear, harder to parse in shell entrypoints. Separate args are more standard for container command args.
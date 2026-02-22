# Implementation Plan: Jolokia Sidecar Injection Webhook

**Branch**: `001-jolokia-sidecar-injection` | **Date**: 2026-02-22 | **Spec**: [spec.md](./spec.md)
**Input**: Feature specification from `/specs/001-jolokia-sidecar-injection/spec.md`

## Summary

Implement a Kubernetes mutating webhook that injects a Jolokia JVM agent sidecar into annotated Pods. The webhook validates `jolokia.horoa.net/*` annotation keys against 58 known Jolokia options plus 7 operator-specific keys, rejects invalid keys, and mutates accepted Pods by injecting a native sidecar init container (`restartPolicy: Always`, KEP-753), an `emptyDir` volume, `shareProcessNamespace: true`, and `SYS_PTRACE` capability. The sidecar image is configurable per-Pod (annotation) and operator-wide (CLI flag). Jolokia options are passed to the sidecar as container command args.

## Technical Context

**Language/Version**: Go 1.25.3 (as specified in `go.mod`)
**Primary Dependencies**: `sigs.k8s.io/controller-runtime` v0.23.1, `k8s.io/api` v0.35.0, `k8s.io/apimachinery` v0.35.0, `k8s.io/client-go` v0.35.0
**Storage**: N/A (no persistent state — webhook is stateless)
**Testing**: Ginkgo v2.27.2 / Gomega v1.38.2 (unit + envtest integration), Kind cluster (e2e via `make test-e2e`)
**Target Platform**: Kubernetes 1.33+ clusters (Linux amd64/arm64 containers)
**Project Type**: Kubernetes operator (admission webhook)
**Performance Goals**: 200ms p99 webhook latency, 128Mi steady-state memory budget (Constitution IV)
**Constraints**: No external network calls during admission, `failurePolicy: Fail`, stateless webhook handler
**Scale/Scope**: Single mutating webhook + single validating webhook on Pod CREATE, supporting up to 10,000 Pods

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | Notes |
|-----------|--------|-------|
| I. Code Quality | ✅ PASS | All code will pass golangci-lint (`.golangci.yml` with 20+ linters). No `//nolint` without justification. |
| II. Testing Standards | ✅ PASS | Three-tier pyramid: unit tests for annotation validation + injection logic, envtest integration for webhook roundtrip, Kind e2e for full lifecycle. Ginkgo/Gomega BDD. |
| III. Operator UX Consistency | ✅ PASS | Annotation-driven opt-in. Clear rejection messages listing invalid keys. Idempotent (detects existing `jolokia-agent` init container). Namespace exclusion via `namespaceSelector`. |
| IV. Performance Requirements | ✅ PASS | Stateless webhook — no external calls, no cache lookups needed. Annotation map iteration is O(n) on a small set. `timeoutSeconds: 10`. |
| V. K8s Best Practices (1.33+) | ✅ PASS | Native sidecar via init container with `restartPolicy: Always`. `admissionReviewVersions: ["v1"]`. `sideEffects: None`. `reinvocationPolicy: IfNeeded`. RBAC least privilege. |
| VI. Security | ✅ PASS | TLS via cert-manager (already wired). No secret logging. Sidecar security context: `readOnlyRootFilesystem: true`, `allowPrivilegeEscalation: false`, `capabilities.drop: ["ALL"]`, `capabilities.add: ["SYS_PTRACE"]`, `runAsNonRoot: true`, `seccompProfile.type: RuntimeDefault`. |
| VII. Observability | ✅ PASS | Structured logging via logr (injection events, rejections). Prometheus metrics for admission counts/latency. Health endpoints already scaffolded. |

**No violations. Gate PASSED.**

## Project Structure

### Documentation (this feature)

```text
specs/001-jolokia-sidecar-injection/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/           # Phase 1 output
│   └── admission.md     # Webhook admission request/response contract
└── tasks.md             # Phase 2 output (/speckit.tasks command)
```

### Source Code (repository root)

```text
cmd/
└── main.go                                  # MODIFY: Add --sidecar-image flag, pass to webhook setup

internal/
├── controller/
│   └── pod_controller.go                    # EXISTS (scaffolded, empty reconciler — not needed for webhook-only feature, keep as-is)
├── webhook/v1/
│   ├── pod_webhook.go                       # MODIFY: Implement PodCustomDefaulter.Default (injection) and PodCustomValidator.ValidateCreate (validation)
│   ├── pod_webhook_test.go                  # MODIFY: Unit tests for injection + validation logic
│   └── webhook_suite_test.go                # EXISTS (envtest webhook test suite setup)
└── jolokia/                                 # CREATE: Domain logic package
    ├── annotations.go                       # CREATE: Annotation parsing, key validation, option extraction
    ├── annotations_test.go                  # CREATE: Table-driven tests for annotation validation
    ├── injector.go                          # CREATE: Pod mutation logic (sidecar container spec builder, volume injection)
    └── injector_test.go                     # CREATE: Unit tests for injection logic

config/
├── webhook/
│   ├── manifests.yaml                       # AUTO-GENERATED from markers (DO NOT EDIT)
│   ├── kustomization.yaml                   # EXISTS
│   └── service.yaml                         # EXISTS
├── default/
│   ├── kustomization.yaml                   # EXISTS (webhook + certmanager already enabled)
│   └── manager_webhook_patch.yaml           # EXISTS
├── rbac/
│   └── role.yaml                            # AUTO-GENERATED from RBAC markers
├── certmanager/                             # EXISTS (cert-manager webhook cert config)
└── manager/
    └── manager.yaml                         # EXISTS (Deployment manifest)

test/e2e/
├── e2e_test.go                              # MODIFY: Add sidecar injection e2e scenarios
└── e2e_suite_test.go                        # EXISTS
```

**Structure Decision**: Standard Kubebuilder single-group layout. New `internal/jolokia/` package isolates domain logic (annotation validation + injection) from webhook plumbing. This keeps `pod_webhook.go` thin (delegates to `jolokia` package) and makes the domain logic independently testable without envtest.

## Complexity Tracking

> No Constitution violations. Table empty.

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| *(none)* | | |

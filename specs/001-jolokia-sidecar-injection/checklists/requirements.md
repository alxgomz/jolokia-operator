# Requirements Checklist: Jolokia Sidecar Injection Webhook

**Purpose**: Validate that the feature specification is complete, unambiguous, and implementable.
**Created**: 2026-02-22
**Feature**: [spec.md](../spec.md)

## Completeness — User Scenarios

- [x] CHK001 All user stories have a priority assigned (P1/P2/P3)
- [x] CHK002 Each user story is independently testable
- [x] CHK003 Each user story has acceptance scenarios in Given/When/Then format
- [x] CHK004 P1 stories cover the minimum viable product (injection + validation + option pass-through)
- [x] CHK005 Edge cases are enumerated and cover boundary conditions (empty values, duplicate injection, system namespaces, existing shareProcessNamespace)
- [x] CHK006 Negative paths are covered (invalid annotations → rejection, no annotations → pass-through)

## Completeness — Functional Requirements

- [x] CHK007 Annotation prefix (`jolokia.horoa.net/`) is explicitly defined
- [x] CHK008 Exhaustive list of valid Jolokia JVM agent option names is provided (58 options, Appendix A)
- [x] CHK009 Operator-specific annotation keys are listed separately with defaults (Appendix B)
- [x] CHK010 Validation behavior is unambiguous: reject on invalid keys, do not validate values
- [x] CHK011 Sidecar injection mechanism is specified (native sidecar: init container with `restartPolicy: Always`)
- [x] CHK012 Required security context (`SYS_PTRACE` capability) is specified
- [x] CHK013 `shareProcessNamespace: true` requirement is specified
- [x] CHK014 Volume name (`jolokia-tmp`), type (`emptyDir`), and default mount path (`/tmp`) are specified
- [x] CHK015 Option pass-through mechanism is described (annotations → sidecar args/env)
- [x] CHK016 Resource annotation mapping to sidecar resources is specified (all 4: limits/requests × cpu/memory)
- [x] CHK017 Target process annotation behavior is specified
- [x] CHK018 Idempotency requirement is specified (skip if already injected)
- [x] CHK019 Opt-in behavior is explicit (no injection without annotations, Constitution Principle III)

## Completeness — Webhook Configuration

- [x] CHK020 `failurePolicy: Fail` is specified with rationale
- [x] CHK021 `sideEffects: None` is specified
- [x] CHK022 `admissionReviewVersions: ["v1"]` is specified (K8s 1.33+)
- [x] CHK023 `timeoutSeconds` recommendation is specified (≤10s)
- [x] CHK024 Namespace exclusion via `namespaceSelector` is mentioned (system namespaces)
- [x] CHK025 Webhook targets Pod CREATE operations

## Completeness — Observability

- [x] CHK026 Structured logging for injection events is required (Pod name, namespace, options)
- [x] CHK027 Structured logging for validation rejections is required (Pod name, namespace, invalid keys)
- [x] CHK028 Prometheus metrics are recommended (admission count, injections, rejections)

## Completeness — Success Criteria

- [x] CHK029 Performance target is measurable (200ms p99 for injection, 50ms p99 for pass-through)
- [x] CHK030 Test coverage target is defined (≥90% unit, envtest for all stories, e2e on Kind)
- [x] CHK031 Concurrency target is defined (100 concurrent admissions)
- [x] CHK032 Memory budget is defined (128Mi per Constitution Principle IV)

## Consistency — Alignment with Constitution

- [x] CHK033 Native sidecar pattern (KEP-753) aligns with Constitution Principle V
- [x] CHK034 Opt-in annotation approach aligns with Constitution Principle III
- [x] CHK035 Performance targets align with Constitution Principle IV
- [x] CHK036 Structured logging aligns with Constitution Principle VII
- [x] CHK037 failurePolicy: Fail aligns with Constitution Principle VI (security)

## Consistency — Alignment with Codebase

- [x] CHK038 Spec references existing scaffold: `PodCustomDefaulter.Default()` in `internal/webhook/v1/pod_webhook.go`
- [x] CHK039 Spec aligns with existing kubebuilder markers (mutating=true, failurePolicy=fail, sideEffects=None, admissionReviewVersions=v1)
- [x] CHK040 Spec does not conflict with existing controller scaffold (`internal/controller/pod_controller.go`)

## Ambiguity Check

- [x] CHK041 No placeholder text remains (all `[NEEDS CLARIFICATION]` resolved or absent)
- [x] CHK042 MUST vs SHOULD usage is consistent (MUST for hard requirements, SHOULD for recommendations)
- [x] CHK043 Default values are specified for all optional annotations
- [x] CHK044 Annotation key casing rule is explicit (case-sensitive, must match Jolokia option name exactly)

## Notes

- Check items off as completed: `[x]`
- All 44 items pass. The spec is complete, unambiguous, and aligned with both the constitution and the existing codebase scaffold.
- No NEEDS CLARIFICATION items remain in the spec.

<!--
Sync Impact Report
==================
- Version change: 0.0.0 (template) → 1.0.0 (initial ratification)
- Modified principles: N/A (all new)
- Added sections:
  - Core Principles (7 principles: Code Quality, Testing Standards,
    Operator UX Consistency, Performance, Kubernetes Best Practices,
    Security, Observability)
  - Kubernetes-Specific Constraints
  - Development Workflow & Quality Gates
  - Governance
- Removed sections: None (template placeholders replaced)
- Templates requiring updates:
  - `.specify/templates/plan-template.md` — ✅ No changes needed
    (Constitution Check section references constitution generically)
  - `.specify/templates/spec-template.md` — ✅ No changes needed
    (requirements/success criteria sections are technology-agnostic)
  - `.specify/templates/tasks-template.md` — ✅ No changes needed
    (task structure is generic; principle-driven phases like
    observability/security fit into Phase N: Polish)
  - `.specify/templates/commands/*.md` — ✅ N/A (no command templates exist)
- Follow-up TODOs: None
-->

# Jolokia Operator Constitution

## Core Principles

### I. Code Quality (NON-NEGOTIABLE)

All Go source code MUST pass the full golangci-lint rule set defined in
`.golangci.yml` with zero violations before merge. Specifically:

- **Static analysis**: `govet`, `staticcheck`, `errcheck`, `ineffassign`,
  `unconvert`, `unparam`, and `unused` MUST report zero findings.
- **Complexity**: `gocyclo` MUST enforce a maximum cyclomatic complexity
  per function. Functions exceeding the threshold MUST be refactored.
- **Style**: `revive` rules (including `comment-spacings` and
  `import-shadowing`) MUST be satisfied. Code MUST be formatted with
  `gofmt` and `goimports`.
- **Kubernetes conventions**: `logcheck` MUST validate that all logging
  calls follow Kubernetes structured logging conventions.
- **Duplication**: `dupl` MUST flag duplicated code blocks; duplicated
  logic MUST be extracted into shared functions.
- **No suppression**: `//nolint` directives are permitted ONLY with an
  accompanying justification comment explaining why the lint rule does
  not apply. Blanket `//nolint` without a specific linter name is
  forbidden.
- **Generated code**: Files matching `zz_generated.*` and
  `config/crd/bases/*.yaml` MUST NOT be manually edited. Run
  `make manifests generate` to regenerate.

**Rationale**: The operator runs as a cluster-wide admission controller.
Code defects can block Pod creation across the entire cluster. Zero
tolerance for static analysis violations prevents classes of bugs that
are trivially detectable.

### II. Testing Standards (NON-NEGOTIABLE)

Every behavioral change MUST be covered by tests at the appropriate
level. The project enforces a three-tier testing pyramid:

- **Unit tests** (Ginkgo/Gomega + envtest): MUST cover all webhook
  defaulting and validation logic, all reconciliation logic, and all
  helper/utility functions. Unit tests run via `make test` and MUST
  pass with zero failures. Coverage MUST NOT decrease on any PR.
- **Integration tests** (envtest with real K8s API server + etcd):
  MUST verify that webhook registration succeeds, that sidecar
  injection mutates Pods correctly, and that the controller reconciles
  expected state. These run as part of `make test`.
- **End-to-end tests** (Kind cluster, `make test-e2e`): MUST validate
  the full deployment lifecycle — CRD installation, controller-manager
  startup, webhook endpoint readiness, cert-manager certificate
  provisioning, metrics endpoint availability, and actual sidecar
  injection on a live cluster.

Testing rules:

- Tests MUST use the Ginkgo/Gomega BDD framework with `Describe`,
  `Context`, `It`, `BeforeEach`, and `AfterEach` blocks.
- Tests MUST NOT depend on external services or network access
  (except e2e tests which require a Kind cluster).
- Test names MUST be descriptive: `"Should inject jolokia sidecar
  when annotation is present"` not `"Test1"`.
- Table-driven tests SHOULD be used for validating multiple input
  combinations in webhook validation logic.
- E2e tests MUST run against an isolated Kind cluster — NEVER against
  a shared or production cluster.

**Rationale**: A mutating webhook that silently corrupts Pod specs can
cause cascading failures. Comprehensive testing at every layer is the
primary defense against regressions.

### III. Operator UX Consistency

The operator MUST provide a consistent, predictable experience for
cluster administrators and application developers:

- **Annotation-driven configuration**: Sidecar injection MUST be
  controlled via well-documented Pod or Namespace annotations. The
  annotation schema MUST be stable across minor versions.
- **Opt-in by default**: The operator MUST NOT inject sidecars into
  Pods unless explicitly requested via annotations. Silent injection
  is forbidden.
- **Clear feedback**: When injection is skipped (missing annotation,
  namespace exclusion, invalid configuration), the webhook MUST log
  the reason at an appropriate verbosity level. When injection fails,
  the webhook MUST return a clear admission rejection message.
- **Idempotency**: Applying the same Pod spec multiple times through
  the webhook MUST produce identical results. The webhook MUST detect
  and skip already-injected sidecars.
- **Namespace exclusion**: The operator MUST support excluding
  namespaces from injection (e.g., `kube-system`,
  `jolokia-operator-system`). System namespaces MUST be excluded by
  default.
- **Kubernetes API conventions**: All status reporting MUST use
  `metav1.Condition`. Log messages MUST follow Kubernetes logging
  style: capital first letter, no trailing period, active voice,
  past tense for completed actions.

**Rationale**: Operators are infrastructure — they must be invisible
when working correctly and immediately informative when something goes
wrong. Consistent UX reduces operational burden.

### IV. Performance Requirements

Admission webhooks sit in the critical path of every Pod creation.
Performance is a functional requirement, not an optimization:

- **Webhook latency**: The mutating webhook MUST respond within
  **200ms p99** under normal load. The webhook timeout MUST be
  configured to **10 seconds** (Kubernetes default is 10s; never
  increase beyond 15s).
- **No external calls**: The webhook handler MUST NOT make network
  calls to external services during admission. All injection
  configuration MUST be pre-loaded or cached.
- **Memory efficiency**: The controller-manager MUST NOT exceed
  **128Mi** memory under steady-state operation with up to 10,000
  Pods. Resource requests/limits MUST be set in the Deployment
  manifest.
- **Startup probe**: The controller-manager MUST expose a health
  endpoint and configure `startupProbe`, `livenessProbe`, and
  `readinessProbe` to prevent traffic before the webhook server is
  ready.
- **Failure policy**: The mutating webhook MUST use
  `failurePolicy: Fail` for production (to prevent uninjected Pods)
  with clear documentation on how to switch to `Ignore` for
  emergency recovery scenarios.
- **Reconciliation efficiency**: The controller MUST use watches
  with label selectors or field selectors to minimize unnecessary
  reconciliation cycles. `RequeueAfter` MUST use exponential backoff
  for retries.

**Rationale**: A slow or failing webhook blocks Pod scheduling
cluster-wide. Performance budgets are enforced as hard requirements
to prevent the operator from becoming a bottleneck.

### V. Kubernetes Best Practices (K8s 1.33+)

The operator targets Kubernetes 1.33 and beyond. All implementations
MUST follow current best practices and leverage GA features:

- **Native sidecar containers (KEP-753)**: Injected sidecars MUST
  use the native sidecar pattern — init containers with
  `restartPolicy: Always`. This is GA in Kubernetes 1.29+ and MUST
  be the only injection mechanism. Legacy regular-container sidecar
  injection is forbidden.
- **Admission review version**: Webhooks MUST use
  `admissionReviewVersions: ["v1"]`. The `v1beta1` version is
  removed in Kubernetes 1.22+ and MUST NOT be referenced.
- **Side effects**: Webhook configurations MUST declare
  `sideEffects: None`. If the webhook has side effects, it MUST
  declare `sideEffects: NoneOnDryRun` and handle dry-run requests.
- **Reinvocation policy**: The mutating webhook MUST set
  `reinvocationPolicy: IfNeeded` to handle interactions with other
  mutating webhooks that may modify the same Pod spec.
- **RBAC least privilege**: RBAC markers MUST request only the
  minimum required permissions. The controller MUST NOT request
  cluster-admin or wildcard (`*`) permissions.
- **Owner references**: When the operator creates secondary
  resources, it MUST set owner references for automatic garbage
  collection via `SetControllerReference`.
- **Pod Security Standards**: The controller-manager Pod MUST comply
  with the `restricted` Pod Security Standard. E2e tests MUST
  enforce this via namespace labeling.
- **Kubebuilder scaffold markers**: `// +kubebuilder:scaffold:*`
  comments MUST NOT be removed. They are required for CLI code
  generation.

**Rationale**: Kubernetes evolves rapidly. Targeting 1.33+ means
using GA features (not alpha/beta) and following current conventions
to ensure forward compatibility and supportability.

### VI. Security

The operator handles admission requests containing full Pod specs,
which may include secrets. Security is non-negotiable:

- **TLS**: All webhook endpoints MUST be served over TLS. Certificate
  management MUST use cert-manager (already scaffolded) for automatic
  rotation.
- **No secret logging**: Webhook handlers MUST NOT log full Pod specs
  at default verbosity. Sensitive fields (environment variables,
  volume mounts with secret references) MUST be redacted in debug
  logs.
- **Container image pinning**: Sidecar container images injected by
  the operator MUST reference images by digest (`@sha256:...`) or
  by an immutable tag. Mutable tags like `:latest` MUST NOT be used
  in production configurations.
- **Security context**: All injected sidecar containers MUST include
  a security context with: `readOnlyRootFilesystem: true`,
  `allowPrivilegeEscalation: false`, `capabilities.drop: ["ALL"]`,
  `runAsNonRoot: true`, and `seccompProfile.type: RuntimeDefault`.
- **Namespace isolation**: The operator MUST NOT process admission
  requests from its own namespace to prevent self-mutation loops.
- **Input validation**: All annotation values and configuration
  inputs MUST be validated and sanitized. Malformed inputs MUST
  result in admission rejection with a descriptive error, not
  silent fallback to defaults.

**Rationale**: A mutating webhook with cluster-wide scope is a
high-value attack surface. Defense-in-depth principles apply at
every layer.

### VII. Observability

The operator MUST be observable in production without requiring
code changes or redeployment:

- **Structured logging**: All log output MUST use the `logr`
  interface via `sigs.k8s.io/controller-runtime/pkg/log`. Log
  messages MUST follow Kubernetes logging conventions (capital
  first letter, no period, key-value pairs).
- **Log verbosity levels**: Level 0 (Info) for lifecycle events
  (startup, shutdown, injection performed). Level 1 for routine
  operations (skipped Pods, cache updates). Level 2+ for debug
  detail (full injection diffs, request/response traces).
- **Metrics**: The controller-manager MUST expose Prometheus
  metrics at `/metrics` including: webhook request count
  (by result: injected/skipped/rejected), webhook latency
  histogram, reconciliation counts, and error rates.
- **Health endpoints**: `/healthz` (liveness) and `/readyz`
  (readiness) MUST be implemented. Readiness MUST report not-ready
  until the webhook server is fully initialized and the TLS
  certificate is loaded.
- **Events**: The operator SHOULD emit Kubernetes Events on
  successful injection and on injection failures, attached to the
  target Pod, to aid in debugging via `kubectl describe pod`.

**Rationale**: Operators run unattended. When sidecar injection
fails silently, the only recourse is observability data. Every
operational question MUST be answerable from logs, metrics, or
events without attaching a debugger.

## Kubernetes-Specific Constraints

These constraints apply to all code in this repository and are
enforced by tooling and review:

- **Go version**: The project MUST use the Go version specified
  in `go.mod` (currently 1.25.3). Upgrades MUST be coordinated
  with controller-runtime compatibility.
- **Kubebuilder CLI**: API scaffolding MUST use `kubebuilder
  create api` and `kubebuilder create webhook`. Manual file
  creation for CRD types or webhook registrations is forbidden.
- **Dependency management**: Direct dependencies MUST be limited
  to: `k8s.io/api`, `k8s.io/apimachinery`, `k8s.io/client-go`,
  `sigs.k8s.io/controller-runtime`, and `github.com/onsi/ginkgo`
  / `github.com/onsi/gomega`. Additional dependencies require
  explicit justification in the PR description.
- **Regeneration after type changes**: After any edit to
  `*_types.go` or kubebuilder markers, `make manifests generate`
  MUST be run and the generated output committed. CI MUST verify
  that generated files are up to date.
- **Controller-runtime patterns**: Controllers MUST use
  `ctrl.NewControllerManagedBy`, webhooks MUST use
  `ctrl.NewWebhookManagedBy`. Direct informer/lister usage is
  forbidden unless justified by a performance requirement.

## Development Workflow & Quality Gates

All changes MUST pass through these gates before merge:

1. **Local validation**: `make lint-fix` and `make test` MUST pass
   locally before pushing.
2. **CI pipeline**: The CI pipeline MUST run, in order:
   - `make manifests generate` — verify generated files are current
   - `make lint` — zero lint violations
   - `make test` — all unit and integration tests pass
   - `make test-e2e` — all e2e tests pass on an isolated Kind cluster
3. **Code review**: Every PR MUST be reviewed by at least one
   maintainer. Reviewers MUST verify:
   - Constitution compliance (principles I–VII)
   - Test coverage for new behavior
   - No `//nolint` without justification
   - No manual edits to generated files
4. **Breaking change protocol**: Changes that alter the annotation
   schema, injection behavior, or webhook configuration MUST include:
   - Migration documentation
   - Deprecation notice (minimum one minor release before removal)
   - Updated e2e tests validating the migration path

## Governance

This constitution is the authoritative source for technical decision-
making in the jolokia-operator project. It supersedes informal
conventions, verbal agreements, and individual preferences.

**Compliance**:
- All pull requests and code reviews MUST verify compliance with
  the principles defined in this document.
- The plan template's "Constitution Check" section MUST reference
  the principles by number (I–VII) and verify each before
  implementation begins.
- Complexity or scope that violates a principle MUST be documented
  in the plan's "Complexity Tracking" table with justification.

**Amendment procedure**:
1. Propose the amendment as a PR modifying this file.
2. The PR description MUST include: which principle is affected,
   the rationale for the change, and the impact assessment.
3. Amendment PRs require approval from at least two maintainers.
4. After merge, all in-progress feature branches MUST be notified
   of the change and updated if affected.

**Versioning policy**:
- **MAJOR**: Removal or redefinition of an existing principle.
- **MINOR**: New principle added or existing principle materially
  expanded.
- **PATCH**: Clarifications, typo fixes, non-semantic refinements.

**Review cadence**: This constitution MUST be reviewed at least once
per Kubernetes minor release cycle to verify alignment with upstream
best practices and deprecations.

**Version**: 1.0.0 | **Ratified**: 2026-02-22 | **Last Amended**: 2026-02-22

# Tasks: Jolokia Sidecar Injection Webhook

**Input**: Design documents from `/specs/001-jolokia-sidecar-injection/`
**Prerequisites**: plan.md ✅, spec.md ✅, research.md ✅, data-model.md ✅, contracts/admission.md ✅, quickstart.md ✅

**Tests**: Required (SC-004: ≥90% unit test coverage, SC-005: envtest integration for all 7 stories + edge cases, SC-006: e2e on Kind).

**Organization**: Tasks grouped by user story. Each story is independently testable.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3)
- Exact file paths included in descriptions

---

## Phase 1: Setup

**Purpose**: Project initialization — create the `internal/jolokia/` domain package and configuration scaffolding.

- [ ] T001 Create `internal/jolokia/` package directory and `doc.go` with package comment in `internal/jolokia/doc.go`
- [ ] T002 [P] Add `--sidecar-image` CLI flag to `cmd/main.go` and pass it to `SetupPodWebhookWithManager` (per R-004)
- [ ] T003 [P] Update `SetupPodWebhookWithManager` signature in `internal/webhook/v1/pod_webhook.go` to accept `defaultImage string` and wire it to `PodCustomDefaulter{DefaultImage: defaultImage}`
- [ ] T004 Update webhook markers in `internal/webhook/v1/pod_webhook.go`: change `verbs=create;update` to `verbs=create` on both mutating and validating markers, add `reinvocationPolicy=ifNeeded` to mutating marker (per R-005)
- [ ] T005 Run `make manifests` to regenerate `config/webhook/manifests.yaml` and `config/rbac/role.yaml` from updated markers

**Checkpoint**: Project compiles (`go build ./...`), webhook markers are correct, `--sidecar-image` flag is wired.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Core domain logic that ALL user stories depend on — annotation parsing, validation sets, and the injection engine skeleton.

**⚠️ CRITICAL**: No user story work can begin until this phase is complete.

- [ ] T006 Implement the 58 valid Jolokia option names as `validJolokiaOptions map[string]struct{}` and the 7 operator-specific keys as `operatorAnnotations map[string]struct{}` in `internal/jolokia/annotations.go` (per data-model.md Entity 2 and spec Appendix A + B)
- [ ] T007 Implement `ParseAnnotations(annotations map[string]string) ParsedAnnotations` function in `internal/jolokia/annotations.go` — extract `jolokia.horoa.net/` prefixed annotations, classify each suffix as jolokia-option / operator-specific / invalid, populate `ParsedAnnotations` struct (per data-model.md Entity 1)
- [ ] T008 Implement `HasJolokiaAnnotations(annotations map[string]string) bool` helper in `internal/jolokia/annotations.go` — returns true if any annotation has the `jolokia.horoa.net/` prefix
- [ ] T009 [P] Write table-driven unit tests for `ParseAnnotations` in `internal/jolokia/annotations_test.go` — cover: all 58 valid options, all 7 operator keys, invalid keys, mixed valid+invalid, empty values, case sensitivity, no jolokia annotations, only operator annotations
- [ ] T010 Implement `InjectSidecar(pod *corev1.Pod, parsed ParsedAnnotations, defaultImage string) InjectionResult` in `internal/jolokia/injector.go` — the core injection function that mutates a Pod in-place: appends native sidecar init container, adds emptyDir volume, sets shareProcessNamespace (per data-model.md Entity 3, R-003)
- [ ] T011 Implement sidecar container spec builder as a helper in `internal/jolokia/injector.go` — builds `corev1.Container` with: name `jolokia-agent`, resolved image, args from JolokiaOptions, `RestartPolicy: Always`, security context (readOnlyRootFilesystem, allowPrivilegeEscalation=false, runAsNonRoot, drop ALL + add SYS_PTRACE, seccompProfile RuntimeDefault), volumeMount for jolokia-tmp (per R-003, R-006, contracts/admission.md)
- [ ] T012 [P] Write unit tests for `InjectSidecar` in `internal/jolokia/injector_test.go` — cover: basic injection (container added, volume added, shareProcessNamespace set), idempotent skip (jolokia-agent already present), no annotations (no-op), security context correctness, volume mount path default `/tmp`

**Checkpoint**: `make test` passes. `ParseAnnotations` correctly classifies all 65 keys. `InjectSidecar` produces correct Pod mutations. ≥90% coverage on `internal/jolokia/`.

---

## Phase 3: User Story 1 — Basic Sidecar Injection (Priority: P1) 🎯 MVP

**Goal**: A Pod annotated with any `jolokia.horoa.net/*` annotation is mutated to include the Jolokia sidecar, emptyDir volume, and security settings.

**Independent Test**: Deploy a Pod with `jolokia.horoa.net/port: "8778"`. Verify sidecar init container, volume, shareProcessNamespace, and SYS_PTRACE.

### Tests for User Story 1

> **NOTE: Write tests FIRST, ensure they FAIL before implementation**

- [ ] T013 [P] [US1] Write envtest integration test in `internal/webhook/v1/pod_webhook_test.go` — test case: Pod with `jolokia.horoa.net/port: "8778"` is mutated to include `jolokia-agent` init container with `restartPolicy: Always`, `jolokia-tmp` emptyDir volume, `shareProcessNamespace: true`, and `SYS_PTRACE` capability
- [ ] T014 [P] [US1] Write envtest integration test in `internal/webhook/v1/pod_webhook_test.go` — test case: Pod with NO `jolokia.horoa.net/*` annotations passes through unmodified (no sidecar, no volume, no shareProcessNamespace change)
- [ ] T015 [P] [US1] Write envtest integration test in `internal/webhook/v1/pod_webhook_test.go` — test case: Pod with shareProcessNamespace already set to `true` — webhook does not conflict (remains true); Pod with shareProcessNamespace set to `false` — webhook overrides to `true`

### Implementation for User Story 1

- [ ] T016 [US1] Implement `PodCustomDefaulter.Default` in `internal/webhook/v1/pod_webhook.go` — call `HasJolokiaAnnotations`, `ParseAnnotations`, check for `InvalidKeys`, call `InjectSidecar`, log injection result (FR-008, FR-009, FR-010, FR-011, FR-015, FR-027)
- [ ] T017 [US1] Verify envtest tests T013–T015 pass

**Checkpoint**: `make test` passes. Pod with jolokia annotations gets sidecar injected. Pod without annotations passes through. US1 acceptance scenarios 1, 2, 3 verified.

---

## Phase 4: User Story 2 — Annotation Validation (Priority: P1)

**Goal**: Pods with invalid `jolokia.horoa.net/*` annotation keys are rejected with a clear error listing the invalid keys.

**Independent Test**: Submit a Pod with `jolokia.horoa.net/foobar: "value"`. Verify admission rejection with error message identifying `foobar`.

### Tests for User Story 2

> **NOTE: Write tests FIRST, ensure they FAIL before implementation**

- [ ] T018 [P] [US2] Write envtest integration test in `internal/webhook/v1/pod_webhook_test.go` — test case: Pod with `jolokia.horoa.net/foobar: "value"` is rejected by validating webhook with error listing `foobar`
- [ ] T019 [P] [US2] Write envtest integration test in `internal/webhook/v1/pod_webhook_test.go` — test case: Pod with mixed valid (`port`) and invalid (`badKey`) annotations is rejected, error lists all invalid keys
- [ ] T020 [P] [US2] Write envtest integration test in `internal/webhook/v1/pod_webhook_test.go` — test case: Pod with only operator-specific annotations (`mnt`, `rsc-limits-cpu`, `rsc-limits-memory`, `rsc-requests-cpu`, `rsc-requests-memory`, `target-process`, `sidecar-image`) is accepted — no validation failure

### Implementation for User Story 2

- [ ] T021 [US2] Implement `PodCustomValidator.ValidateCreate` in `internal/webhook/v1/pod_webhook.go` — call `ParseAnnotations`, if `InvalidKeys` non-empty return field errors listing all invalid keys (FR-003, FR-006, FR-028)
- [ ] T022 [US2] Ensure `PodCustomDefaulter.Default` also rejects invalid keys before injection (defense-in-depth per R-002) — this should already be in T016 but verify the error path
- [ ] T023 [US2] Verify envtest tests T018–T020 pass

**Checkpoint**: `make test` passes. Invalid annotation keys cause rejection with descriptive error. Operator-specific annotations pass validation. US2 acceptance scenarios 1, 2, 3, 4 verified.

---

## Phase 5: User Story 3 — Jolokia Agent Option Pass-Through (Priority: P1)

**Goal**: Valid Jolokia annotation values are passed to the sidecar container as `key=value` args.

**Independent Test**: Submit a Pod with `jolokia.horoa.net/port: "9090"` and `jolokia.horoa.net/host: "0.0.0.0"`. Verify sidecar `args` include `port=9090` and `host=0.0.0.0`.

### Tests for User Story 3

> **NOTE: Write tests FIRST, ensure they FAIL before implementation**

- [ ] T024 [P] [US3] Write unit test in `internal/jolokia/injector_test.go` — verify `InjectSidecar` builds container args as `key=value` from `ParsedAnnotations.JolokiaOptions` (multiple options, sorted deterministically)
- [ ] T025 [P] [US3] Write envtest integration test in `internal/webhook/v1/pod_webhook_test.go` — test case: Pod with `port: "9090"` and `host: "0.0.0.0"` annotations → sidecar args contain `host=0.0.0.0` and `port=9090`
- [ ] T026 [P] [US3] Write envtest integration test in `internal/webhook/v1/pod_webhook_test.go` — test case: Pod with only operator annotations (`mnt: "/data"`) → sidecar args contain NO jolokia options (only operator behavior changes)

### Implementation for User Story 3

- [ ] T027 [US3] Implement Jolokia args building in `internal/jolokia/injector.go` — sort option keys alphabetically for deterministic output, format as `key=value`, set as sidecar container `Args` (FR-013, R-007)
- [ ] T028 [US3] Verify unit test T024 and envtest tests T025–T026 pass

**Checkpoint**: `make test` passes. Jolokia options flow from annotations to sidecar args. Operator-specific annotations do not appear in args. US3 acceptance scenarios 1, 2 verified.

---

## Phase 6: User Story 4 — Custom Mount Path (Priority: P2)

**Goal**: `jolokia.horoa.net/mnt` annotation controls the volume mount path in the sidecar.

**Independent Test**: Submit a Pod with `jolokia.horoa.net/mnt: "/opt/jolokia"`. Verify `jolokia-tmp` volume mount is at `/opt/jolokia`.

### Tests for User Story 4

- [ ] T029 [P] [US4] Write unit test in `internal/jolokia/injector_test.go` — verify `InjectSidecar` uses `ParsedAnnotations.MountPath` for volume mount path
- [ ] T030 [P] [US4] Write envtest integration test in `internal/webhook/v1/pod_webhook_test.go` — test case: Pod with `mnt: "/opt/jolokia"` → sidecar volume mount at `/opt/jolokia`; Pod without `mnt` → sidecar volume mount at `/tmp`

### Implementation for User Story 4

- [ ] T031 [US4] Ensure `ParseAnnotations` extracts `mnt` annotation value into `ParsedAnnotations.MountPath` with default `/tmp` in `internal/jolokia/annotations.go` (should already exist from T007 — verify and adjust if needed)
- [ ] T032 [US4] Ensure `InjectSidecar` uses `parsed.MountPath` for the volume mount path in `internal/jolokia/injector.go` (should already exist from T011 — verify and adjust if needed)
- [ ] T033 [US4] Verify tests T029–T030 pass

**Checkpoint**: `make test` passes. Custom mount path works. Default `/tmp` fallback works. US4 acceptance scenarios 1, 2 verified.

---

## Phase 7: User Story 5 — Sidecar Resource Allocation (Priority: P2)

**Goal**: Resource annotations (`rsc-limits-cpu`, `rsc-limits-memory`, `rsc-requests-cpu`, `rsc-requests-memory`) control the sidecar's resource requests and limits.

**Independent Test**: Submit a Pod with all four resource annotations. Verify sidecar container `resources` field matches.

### Tests for User Story 5

- [ ] T034 [P] [US5] Write unit test in `internal/jolokia/injector_test.go` — verify `InjectSidecar` sets sidecar `resources.limits` and `resources.requests` from `ParsedAnnotations.Resources`
- [ ] T035 [P] [US5] Write envtest integration test in `internal/webhook/v1/pod_webhook_test.go` — test cases: Pod with all 4 resource annotations → correct limits/requests; Pod with partial (only limits) → only limits set; Pod with no resource annotations → no explicit resources on sidecar

### Implementation for User Story 5

- [ ] T036 [US5] Implement resource allocation in the sidecar container builder in `internal/jolokia/injector.go` — parse `SidecarResources` fields into `corev1.ResourceRequirements`, only set fields that are non-empty (FR-017, FR-018, FR-019, FR-020)
- [ ] T037 [US5] Verify tests T034–T035 pass

**Checkpoint**: `make test` passes. Resource annotations control sidecar resources. Missing annotations leave resources unset. US5 acceptance scenarios 1, 2, 3 verified.

---

## Phase 8: User Story 6 — Target Process Selection (Priority: P2)

**Goal**: `jolokia.horoa.net/target-process` annotation (numeric PID or Java class name) is passed to the sidecar as a container arg.

**Independent Test**: Submit a Pod with `jolokia.horoa.net/target-process: "org.apache.catalina.startup.Bootstrap"`. Verify sidecar args include `target-process=org.apache.catalina.startup.Bootstrap`.

### Tests for User Story 6

- [ ] T038 [P] [US6] Write unit test in `internal/jolokia/injector_test.go` — verify `InjectSidecar` adds `target-process=<value>` to sidecar args when `ParsedAnnotations.TargetProcess` is set (test both class name and numeric PID)
- [ ] T039 [P] [US6] Write envtest integration test in `internal/webhook/v1/pod_webhook_test.go` — test cases: Pod with `target-process: "org.apache.catalina.startup.Bootstrap"` → arg present; Pod with `target-process: "42"` → arg present; Pod without target-process → no target-process arg

### Implementation for User Story 6

- [ ] T040 [US6] Implement target-process arg injection in `internal/jolokia/injector.go` — if `parsed.TargetProcess` is non-empty, append `target-process=<value>` to sidecar container args (FR-014)
- [ ] T041 [US6] Verify tests T038–T039 pass

**Checkpoint**: `make test` passes. Target process (class name or PID) flows to sidecar args. Absent annotation means no target-process arg. US6 acceptance scenarios 1, 2, 3 verified.

---

## Phase 9: User Story 7 — Idempotent Re-injection Prevention (Priority: P3)

**Goal**: Pods that already have the `jolokia-agent` init container are NOT injected a second time.

**Independent Test**: Submit a Pod that already contains an init container named `jolokia-agent`. Verify no duplicate sidecar is added.

### Tests for User Story 7

- [ ] T042 [P] [US7] Write unit test in `internal/jolokia/injector_test.go` — verify `InjectSidecar` returns `Skipped: true` when Pod already has init container named `jolokia-agent`
- [ ] T043 [P] [US7] Write envtest integration test in `internal/webhook/v1/pod_webhook_test.go` — test case: Pod pre-populated with `jolokia-agent` init container + jolokia annotations → webhook does NOT add a second sidecar, Pod unchanged

### Implementation for User Story 7

- [ ] T044 [US7] Implement idempotency check in `InjectSidecar` in `internal/jolokia/injector.go` — scan `pod.Spec.InitContainers` for name `jolokia-agent`, if found set `Skipped: true` with reason and return early (FR-016)
- [ ] T045 [US7] Verify tests T042–T043 pass

**Checkpoint**: `make test` passes. Already-injected Pods are not double-injected. US7 acceptance scenario 1 verified.

---

## Phase 10: Polish & Cross-Cutting Concerns

**Purpose**: Namespace exclusion, sidecar image override, observability, edge cases, and e2e validation.

- [ ] T046 [P] Create `config/webhook/namespace_selector_patch.yaml` with JSON patch to add `namespaceSelector` excluding `kube-system`, `kube-public`, `kube-node-lease`, `jolokia-operator-system` (per R-005, FR-023)
- [ ] T047 [P] Update `config/webhook/kustomization.yaml` to include `namespace_selector_patch.yaml` as a JSON6902 patch targeting the `MutatingWebhookConfiguration` and `ValidatingWebhookConfiguration`
- [ ] T048 Implement sidecar image resolution in `internal/jolokia/injector.go` — precedence: `ParsedAnnotations.SidecarImage` > `defaultImage` param > hardcoded `ghcr.io/alxgomz/jolokia-agent:2` (FR-016a)
- [ ] T049 [P] Write unit test for sidecar image resolution in `internal/jolokia/injector_test.go` — test all 3 precedence levels: annotation set, annotation empty + default flag set, both empty → hardcoded default
- [ ] T050 [P] Write envtest integration test for sidecar image override in `internal/webhook/v1/pod_webhook_test.go` — Pod with `sidecar-image` annotation → sidecar uses annotation image; Pod without → uses operator default
- [ ] T051 [P] Write envtest integration test for edge cases in `internal/webhook/v1/pod_webhook_test.go` — Pod with empty annotation value (`port: ""`): accepted; Pod with multiple existing init containers: sidecar appended without disruption; annotation key casing: `agentContext` accepted, `agentcontext` rejected
- [ ] T052 Add structured logging to `PodCustomDefaulter.Default` and `PodCustomValidator.ValidateCreate` in `internal/webhook/v1/pod_webhook.go` — log injection events, idempotent skips, and validation rejections with pod name, namespace, and options/invalid keys (FR-027, FR-028)
- [ ] T053 Run `make manifests generate` to regenerate all auto-generated files
- [ ] T054 Run `make lint-fix` to auto-fix code style across all changed files
- [ ] T055 Run `make test` and verify all unit + envtest integration tests pass with ≥90% coverage on `internal/jolokia/`
- [ ] T056 Create sample Pod manifest at `config/samples/pod-with-jolokia.yaml` — a Pod annotated with `jolokia.horoa.net/port: "8778"` and `jolokia.horoa.net/host: "0.0.0.0"` for manual testing (per quickstart.md)
- [ ] T057 Write e2e test scenarios in `test/e2e/e2e_test.go` — on a Kind cluster: (1) Pod with valid annotations gets sidecar injected, (2) Pod with invalid annotation is rejected, (3) Pod without annotations passes through, (4) Pod with resource + mount + target-process annotations gets fully configured sidecar (SC-006)
- [ ] T058 Run `make test-e2e` on a dedicated Kind cluster and verify all e2e scenarios pass

**Checkpoint**: All tests pass (unit, envtest, e2e). Lint clean. Namespace exclusion configured. Sidecar image override works at all 3 levels. Sample manifests present. SC-001 through SC-008 verified.

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — can start immediately
- **Foundational (Phase 2)**: Depends on Phase 1 completion — BLOCKS all user stories
- **User Stories (Phases 3–9)**: All depend on Phase 2 completion
  - US1 (Phase 3): No story dependencies
  - US2 (Phase 4): No story dependencies (validation is independent of injection wiring)
  - US3 (Phase 5): Depends on US1 (needs working injection to verify args)
  - US4 (Phase 6): Depends on US1 (needs working injection to verify mount path)
  - US5 (Phase 7): Depends on US1 (needs working injection to verify resources)
  - US6 (Phase 8): Depends on US1 (needs working injection to verify target-process arg)
  - US7 (Phase 9): Depends on US1 (needs working injection to test idempotency)
- **Polish (Phase 10)**: Depends on all user story phases being complete

### Within Each User Story

- Tests MUST be written and FAIL before implementation
- Implementation completes the story
- Story checkpoint validates before moving on

### Parallel Opportunities

- **Phase 1**: T002 and T003 can run in parallel (different files)
- **Phase 2**: T009 and T012 can run in parallel (different test files from implementation)
- **Phases 3+4**: US1 and US2 can run in parallel (injection vs validation — independent concerns)
- **Phase 10**: T046, T047, T049, T050, T051 can run in parallel (different files)

---

## Parallel Example: Phase 2 (Foundational)

```
# These can run in parallel (different files):
Task T009: "Table-driven unit tests for ParseAnnotations in internal/jolokia/annotations_test.go"
Task T012: "Unit tests for InjectSidecar in internal/jolokia/injector_test.go"

# These must be sequential (same file, dependencies):
Task T006 → T007 → T008  (annotations.go: sets → parser → helper)
Task T010 → T011          (injector.go: injection function → container builder)
```

---

## Implementation Strategy

### MVP First (User Stories 1 + 2 + 3)

1. Complete Phase 1: Setup
2. Complete Phase 2: Foundational (CRITICAL — blocks all stories)
3. Complete Phase 3: US1 — Basic Sidecar Injection
4. Complete Phase 4: US2 — Annotation Validation
5. Complete Phase 5: US3 — Option Pass-Through
6. **STOP and VALIDATE**: `make test` passes, core injection + validation + args work end-to-end
7. The operator is usable at this point — P1 stories deliver core value

### Incremental Delivery

1. Setup + Foundational → Foundation ready
2. US1 + US2 + US3 → Core MVP (test independently) ✅
3. US4 (mount path) → Enhanced flexibility ✅
4. US5 (resources) → Production readiness ✅
5. US6 (target process) → Multi-process support ✅
6. US7 (idempotency) → Safety net ✅
7. Polish → Namespace exclusion, observability, e2e, samples ✅

---

## Notes

- [P] tasks = different files, no dependencies
- [US*] label maps task to specific user story for traceability
- Each user story is independently testable after Phase 2 foundational work
- `make manifests generate` must run after marker changes (T004, T053)
- `make lint-fix` must run before final test pass (T054)
- E2E tests (T057–T058) require a dedicated Kind cluster — do NOT run against dev/prod
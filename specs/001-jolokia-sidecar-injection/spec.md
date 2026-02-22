# Feature Specification: Jolokia Sidecar Injection Webhook

**Feature Branch**: `001-jolokia-sidecar-injection`  
**Created**: 2026-02-22  
**Status**: Draft  
**Input**: User description: "Develop a Kubernetes mutating webhook that watches Pods for `jolokia.horoa.net/*` annotations, validates them against official Jolokia JVM agent options, and injects a sidecar container with the Jolokia agent plus supporting resources (emptyDir volume, security context, shared process namespace)."

## Clarifications

### Session 2026-02-22

- Q: What container image should the injected Jolokia sidecar use? → A: The sidecar container image is configurable at two levels: (1) per-Pod via the annotation `jolokia.horoa.net/sidecar-image`, and (2) operator-wide via the `--sidecar-image` command-line argument. If neither is set, the default is `ghcr.io/alxgomz/jolokia-agent:2`. Pod annotation takes precedence over operator default.
 Q: What should the injected sidecar container be named? → A: The canonical sidecar init container name is `jolokia-agent`. This name is used for idempotency detection (FR-016) and must not collide with user-defined containers.
 Q: How should Jolokia options be passed to the sidecar container? → A: Via container command args (e.g., `args: ["--port=8778", "--host=0.0.0.0"]`). This is explicit in the Pod spec and follows standard Kubernetes patterns.
 Q: What are the valid formats for `jolokia.horoa.net/target-process`? → A: The value MUST be either a numeric PID (e.g., `1`, `42`) or a Java main class name as it would appear in the output of the Jolokia agent's `list` command (e.g., `org.apache.catalina.startup.Bootstrap`).

## User Scenarios & Testing *(mandatory)*

<!--
  User stories are prioritized as independent, testable slices.
  P1 is the core injection path — without it, nothing works.
  Each subsequent story adds a layer of functionality.
-->

### User Story 1 — Basic Sidecar Injection (Priority: P1)

A platform engineer annotates a Pod (or Pod template in a Deployment) with `jolokia.horoa.net/port: "8778"` to enable Jolokia monitoring. When the Pod is created, the webhook mutates it to inject a Jolokia agent sidecar, an `emptyDir` volume, and the required security settings — without the user needing to manually modify their Pod spec.

**Why this priority**: This is the core value proposition. Without injection, the operator has no purpose. Every other story depends on a working injection path.

**Independent Test**: Deploy a Pod with a single `jolokia.horoa.net/port: "8778"` annotation. Verify the Pod spec is mutated to include the sidecar init container (with `restartPolicy: Always`), the `jolokia-tmp` emptyDir volume, `shareProcessNamespace: true`, and the `SYS_PTRACE` capability on the sidecar.

**Acceptance Scenarios**:

1. **Given** a Pod spec with annotation `jolokia.horoa.net/port: "8778"`, **When** the Pod is submitted to the API server, **Then** the webhook injects a sidecar init container with `restartPolicy: Always`, an emptyDir volume named `jolokia-tmp` mounted at `/tmp`, sets `shareProcessNamespace: true` on the Pod, and adds `SYS_PTRACE` to the sidecar's security context capabilities.
2. **Given** a Pod spec with NO `jolokia.horoa.net/*` annotations, **When** the Pod is submitted, **Then** the webhook does NOT modify the Pod spec (pass-through).
3. **Given** a Deployment whose Pod template has `jolokia.horoa.net/port: "8778"`, **When** a Pod is created from that template, **Then** the injection occurs on the resulting Pod.

---

### User Story 2 — Annotation Validation (Priority: P1)

A developer adds an annotation `jolokia.horoa.net/invalidOption: "value"` to a Pod. The webhook rejects the Pod creation with a clear error message indicating `invalidOption` is not a valid Jolokia JVM agent option, preventing misconfigured Pods from running.

**Why this priority**: Validation is inseparable from injection — accepting invalid options silently would lead to broken Jolokia agents at runtime with no feedback to the user. This must ship with P1.

**Independent Test**: Submit a Pod with an invalid annotation key (e.g., `jolokia.horoa.net/foobar: "value"`) and verify the admission response rejects it with a message listing the invalid key.

**Acceptance Scenarios**:

1. **Given** a Pod with annotation `jolokia.horoa.net/foobar: "value"`, **When** submitted, **Then** the webhook rejects the request with an error message identifying `foobar` as an invalid Jolokia option.
2. **Given** a Pod with valid annotations `jolokia.horoa.net/port: "8778"` and `jolokia.horoa.net/host: "0.0.0.0"`, **When** submitted, **Then** the webhook accepts and injects.
3. **Given** a Pod with a mix of valid (`jolokia.horoa.net/port: "8778"`) and invalid (`jolokia.horoa.net/badKey: "x"`) annotations, **When** submitted, **Then** the webhook rejects the request, listing all invalid keys.
4. **Given** a Pod with operator-specific annotations (`jolokia.horoa.net/mnt`, `jolokia.horoa.net/rsc-limits-cpu`, `jolokia.horoa.net/rsc-limits-memory`, `jolokia.horoa.net/rsc-requests-cpu`, `jolokia.horoa.net/rsc-requests-memory`, `jolokia.horoa.net/target-process`), **When** submitted, **Then** the webhook accepts them — they MUST NOT cause a validation failure.

---

### User Story 3 — Jolokia Agent Option Pass-Through (Priority: P1)

A developer annotates a Pod with `jolokia.horoa.net/port: "9090"` and `jolokia.horoa.net/host: "0.0.0.0"`. The injected sidecar container receives these as container command args so the Jolokia agent starts with `port=9090,host=0.0.0.0`.

**Why this priority**: Options must flow from annotations to the sidecar for the agent to be configurable. Injection without configuration pass-through is not useful.

**Independent Test**: Submit a Pod with multiple Jolokia annotations. Inspect the injected sidecar container's `args` field and verify each annotation value is passed as the corresponding Jolokia agent option.

**Acceptance Scenarios**:

1. **Given** a Pod with annotations `jolokia.horoa.net/port: "9090"` and `jolokia.horoa.net/host: "0.0.0.0"`, **When** injected, **Then** the sidecar container's `args` include `port=9090` and `host=0.0.0.0` as Jolokia agent arguments.
2. **Given** a Pod with only operator-specific annotations (e.g., `jolokia.horoa.net/mnt: "/data"`), **When** injected, **Then** no Jolokia agent options are passed (only the mount path changes).

---

### User Story 4 — Custom Mount Path (Priority: P2)

A developer sets `jolokia.horoa.net/mnt: "/opt/jolokia"` to control where the shared emptyDir volume is mounted in both the sidecar and the main container context.

**Why this priority**: Default `/tmp` works for most cases. Custom mount is a convenience feature for Pods where `/tmp` is already in use or read-only.

**Independent Test**: Submit a Pod with `jolokia.horoa.net/mnt: "/opt/jolokia"` and verify the `jolokia-tmp` volume is mounted at `/opt/jolokia` in the sidecar container.

**Acceptance Scenarios**:

1. **Given** a Pod with `jolokia.horoa.net/mnt: "/opt/jolokia"`, **When** injected, **Then** the `jolokia-tmp` volume mount path in the sidecar is `/opt/jolokia`.
2. **Given** a Pod with no `jolokia.horoa.net/mnt` annotation, **When** injected, **Then** the `jolokia-tmp` volume mount defaults to `/tmp`.

---

### User Story 5 — Sidecar Resource Allocation (Priority: P2)

A platform engineer sets resource annotations (`jolokia.horoa.net/rsc-limits-cpu: "200m"`, `jolokia.horoa.net/rsc-limits-memory: "128Mi"`, `jolokia.horoa.net/rsc-requests-cpu: "50m"`, `jolokia.horoa.net/rsc-requests-memory: "64Mi"`) to control the injected sidecar's resource requests and limits.

**Why this priority**: Resource control is important for production but not required for basic functionality. Without these annotations, sidecar runs with no resource constraints (or operator-defined defaults).

**Independent Test**: Submit a Pod with all four resource annotations and verify the sidecar container's `resources` field matches.

**Acceptance Scenarios**:

1. **Given** a Pod with `jolokia.horoa.net/rsc-limits-cpu: "200m"` and `jolokia.horoa.net/rsc-limits-memory: "128Mi"`, **When** injected, **Then** the sidecar container has `resources.limits.cpu: 200m` and `resources.limits.memory: 128Mi`.
2. **Given** a Pod with `jolokia.horoa.net/rsc-requests-cpu: "50m"` and `jolokia.horoa.net/rsc-requests-memory: "64Mi"`, **When** injected, **Then** the sidecar container has `resources.requests.cpu: 50m` and `resources.requests.memory: 64Mi`.
3. **Given** a Pod with no resource annotations, **When** injected, **Then** the sidecar container has no explicit resource requests or limits set (unless operator-level defaults are defined).

---

### User Story 6 — Target Process Selection (Priority: P2)

A developer sets `jolokia.horoa.net/target-process: "org.apache.catalina.startup.Bootstrap"` to tell the sidecar which process in the main container to attach the Jolokia agent to. The value MUST be either a numeric PID (e.g., `1`) or a Java main class name as it would appear in the output of the Jolokia agent's `list` command (e.g., `org.apache.catalina.startup.Bootstrap`).

**Why this priority**: In containers running a single Java process, auto-detection may suffice. Explicit targeting is needed when the main container runs multiple processes or uses a wrapper script.

**Independent Test**: Submit a Pod with the `target-process` annotation and verify the sidecar container receives this as a configuration parameter.

**Acceptance Scenarios**:

1. **Given** a Pod with `jolokia.horoa.net/target-process: "org.apache.catalina.startup.Bootstrap"`, **When** injected, **Then** the sidecar container receives `org.apache.catalina.startup.Bootstrap` as the target process identifier in its container args.
2. **Given** a Pod with `jolokia.horoa.net/target-process: "42"`, **When** injected, **Then** the sidecar container receives `42` as the target process PID in its container args.
3. **Given** a Pod without `jolokia.horoa.net/target-process`, **When** injected, **Then** the sidecar uses its default process discovery mechanism.

---

### User Story 7 — Idempotent Re-injection Prevention (Priority: P3)

A Pod that has already been injected (e.g., due to a restart or re-admission) is not injected a second time. The webhook detects prior injection and skips mutation.

**Why this priority**: Idempotency is a best practice but is a safety net, not a primary flow. Most Pods are admitted once.

**Independent Test**: Submit a Pod that already contains a container named `jolokia-agent` (or bears the injection marker). Verify the webhook does not add a duplicate sidecar.

**Acceptance Scenarios**:

1. **Given** a Pod that already has the Jolokia sidecar init container, **When** re-admitted through the webhook, **Then** the webhook does NOT add a second sidecar and returns the Pod unchanged.

---

### Edge Cases

- **Pod with only operator-specific annotations and no Jolokia config annotations**: Injection MUST still occur (the annotations `mnt`, `rsc-*`, `target-process` are sufficient to trigger injection since they carry the `jolokia.horoa.net/` prefix).
- **Pod in a system namespace (e.g., `kube-system`)**: Webhook SHOULD be configured with `namespaceSelector` to exclude system namespaces, preventing accidental injection into control plane components.
- **Pod with annotation value containing special characters**: Values are passed as-is to the Jolokia agent. The webhook does not validate annotation values — only keys.
- **Annotation key casing**: Annotation keys after the prefix are case-sensitive and must match the Jolokia option name exactly (e.g., `agentContext` not `agentcontext`).
- **Multiple init containers already present**: The Jolokia sidecar init container MUST be appended without disrupting existing init containers.
- **Pod with `shareProcessNamespace` already set to `true`**: The webhook MUST NOT overwrite or conflict — it should be a no-op for this field if already `true`.
- **Pod with `shareProcessNamespace` set to `false` explicitly**: The webhook MUST set it to `true` (required for agent attachment). This override should be documented as expected behavior.
- **Empty annotation value** (e.g., `jolokia.horoa.net/port: ""`): The webhook MUST accept it — empty values are valid and mean "use Jolokia default".

## Requirements *(mandatory)*

### Functional Requirements

#### Annotation Detection & Validation

- **FR-001**: The webhook MUST intercept Pod CREATE operations via a `MutatingWebhookConfiguration`.
- **FR-002**: The webhook MUST detect annotations with the prefix `jolokia.horoa.net/` on incoming Pod specs.
- **FR-003**: The webhook MUST validate that the annotation key suffix (after `jolokia.horoa.net/`) is either a valid Jolokia JVM agent option OR a recognized operator-specific annotation.
- **FR-004**: Valid Jolokia JVM agent options are (exhaustive list from Jolokia 2.x documentation): `agentContext`, `agentDescription`, `agentId`, `allowDnsReverseLookup`, `allowErrorDetails`, `authClass`, `authIgnoreCerts`, `authMode`, `authPrincipalSpec`, `authUrl`, `authenticator`, `backlog`, `caCert`, `canonicalNaming`, `clientPrincipal`, `dateFormat`, `dateFormatTimeZone`, `debug`, `debugMaxEntries`, `detectorOptions`, `disableDetectors`, `disabledServices`, `discoveryAgentUrl`, `discoveryEnabled`, `enabledServices`, `executor`, `extendedClientCheck`, `historyMaxEntries`, `host`, `includeRequest`, `includeStackTrace`, `jsr160ProxyAllowedTargets`, `keyManagerAlgorithm`, `keyStoreProvider`, `keystore`, `keystorePassword`, `keystoreType`, `logHandlerClass`, `logHandlerName`, `maxCollectionSize`, `maxDepth`, `maxObjects`, `mbeanQualifier`, `mimeType`, `multicastGroup`, `multicastPort`, `password`, `policyLocation`, `port`, `protocol`, `realm`, `registerWhiteboardServlet`, `restrictorClass`, `serializeException`, `serializeLong`, `threadNr`, `useSslClientAuthentication`, `user`, `useRestrictorService`.
 **FR-005**: Recognized operator-specific annotations (not Jolokia options but valid under the prefix) are: `mnt`, `rsc-limits-cpu`, `rsc-limits-memory`, `rsc-requests-cpu`, `rsc-requests-memory`, `target-process`, `sidecar-image`.
- **FR-006**: The webhook MUST reject Pods that contain any `jolokia.horoa.net/*` annotation with an unrecognized key suffix. The rejection response MUST list all invalid keys.
- **FR-007**: The webhook MUST NOT validate annotation values — only keys. Values are passed through as-is.

#### Sidecar Injection

- **FR-008**: The webhook MUST inject a sidecar container as a native sidecar (init container with `restartPolicy: Always`) per KEP-753, GA in Kubernetes 1.29+.
- **FR-009**: The injected sidecar container MUST have the `SYS_PTRACE` capability in its security context to enable process attachment.
- **FR-010**: The webhook MUST set `shareProcessNamespace: true` on the Pod spec to allow the sidecar to see processes in the main container's PID namespace.
- **FR-011**: The webhook MUST inject an `emptyDir` volume named `jolokia-tmp`.
- **FR-012**: The `jolokia-tmp` volume MUST be mounted in the sidecar container at the path specified by the `jolokia.horoa.net/mnt` annotation, defaulting to `/tmp` if the annotation is absent.
 **FR-013**: All valid Jolokia JVM agent options from annotations MUST be passed to the sidecar container as container command args (e.g., `args: ["port=9090", "host=0.0.0.0"]`) so the Jolokia agent starts with those options.
 **FR-014**: The `jolokia.horoa.net/target-process` annotation value, if present, MUST be either a numeric PID or a Java main class name (as returned by the Jolokia agent `list` command). The value MUST be passed to the sidecar container as a container command arg so it knows which process to attach to.
- **FR-015**: The webhook MUST NOT inject if the Pod does not have any `jolokia.horoa.net/*` annotations (opt-in only, per Constitution Principle III).
 **FR-016**: The webhook MUST be idempotent — if a Pod already contains the Jolokia sidecar (detected by the presence of an init container named `jolokia-agent`), injection MUST be skipped.
 **FR-016a**: The injected sidecar container image MUST be resolved in the following precedence order: (1) the `jolokia.horoa.net/sidecar-image` Pod annotation if present, (2) the operator-level `--sidecar-image` command-line argument if set, (3) the hardcoded default `ghcr.io/alxgomz/jolokia-agent:2`.
 **FR-016b**: The operator MUST accept a `--sidecar-image` command-line argument to configure the default sidecar container image at startup.

#### Resource Management

- **FR-017**: If `jolokia.horoa.net/rsc-limits-cpu` is set, the sidecar container's `resources.limits.cpu` MUST be set to the annotation value.
- **FR-018**: If `jolokia.horoa.net/rsc-limits-memory` is set, the sidecar container's `resources.limits.memory` MUST be set to the annotation value.
- **FR-019**: If `jolokia.horoa.net/rsc-requests-cpu` is set, the sidecar container's `resources.requests.cpu` MUST be set to the annotation value.
- **FR-020**: If `jolokia.horoa.net/rsc-requests-memory` is set, the sidecar container's `resources.requests.memory` MUST be set to the annotation value.

#### Webhook Configuration

- **FR-021**: The webhook MUST be registered as a `MutatingWebhookConfiguration` targeting Pod CREATE operations.
- **FR-022**: The webhook `failurePolicy` MUST be `Fail` — if the webhook is unavailable, Pod creation must be blocked to prevent un-validated Pods from running without injection.
- **FR-023**: The webhook SHOULD use a `namespaceSelector` to exclude system namespaces (e.g., `kube-system`, `kube-public`, the operator's own namespace) from interception.
- **FR-024**: The webhook MUST use `admissionReviewVersions: ["v1"]` (per Constitution Principle V for K8s 1.33+).
- **FR-025**: The webhook `sideEffects` MUST be `None`.
- **FR-026**: The webhook `timeoutSeconds` SHOULD be set to 10 seconds or less (per Constitution Principle IV: 200ms p99 latency target).

#### Observability

- **FR-027**: The webhook MUST log each injection at `Info` level with structured fields: Pod name, namespace, and the list of Jolokia options applied.
- **FR-028**: The webhook MUST log validation rejections at `Info` level with the Pod name, namespace, and the invalid annotation keys.
- **FR-029**: The webhook SHOULD expose Prometheus metrics for: total admission requests, injections performed, and validation rejections.

### Key Entities

- **Jolokia Annotation**: A Kubernetes annotation on a Pod with prefix `jolokia.horoa.net/`. The key suffix maps to either a Jolokia JVM agent option or an operator-specific control annotation. The value is an opaque string passed through to the sidecar.
 **Jolokia Sidecar**: A native sidecar init container named `jolokia-agent` (`restartPolicy: Always`) injected into the Pod. It runs the Jolokia JVM agent and attaches to the main container's JVM process via the shared PID namespace. Requires `SYS_PTRACE` capability.
 **Injection Marker**: The presence of an init container named `jolokia-agent` in the Pod spec. The webhook checks for this name to prevent duplicate injection on re-admission.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A Pod annotated with `jolokia.horoa.net/port: "8778"` is mutated within 200ms (p99) to include the sidecar, volume, and security settings.
- **SC-002**: A Pod with an invalid annotation key (e.g., `jolokia.horoa.net/notAnOption: "x"`) is rejected with a clear error message identifying the invalid key.
- **SC-003**: A Pod with no `jolokia.horoa.net/*` annotations passes through the webhook unmodified in under 50ms (p99).
- **SC-004**: Unit test coverage for annotation validation logic is ≥90%.
- **SC-005**: Integration tests (envtest) cover all 7 user stories and all edge cases listed above.
- **SC-006**: E2E tests on a Kind cluster confirm injection works end-to-end: Pod is mutated, sidecar starts, Jolokia agent endpoint responds on the configured port.
- **SC-007**: The webhook handles 100 concurrent Pod admissions without errors or timeouts.
- **SC-008**: The operator's memory footprint stays within the 128Mi budget defined in Constitution Principle IV during sustained admission traffic.

## Appendix A — Valid Jolokia JVM Agent Option Names

The following is the exhaustive list of valid Jolokia 2.x JVM agent configuration option names, sourced from the [official documentation](https://jolokia.org/reference/html/manual/agents.html#jvm-agent). This list includes both JVM-agent-specific options (Table 5) and common servlet init parameters (Table 1) that the JVM agent also accepts.

```
agentContext          agentDescription      agentId
allowDnsReverseLookup allowErrorDetails     authClass
authIgnoreCerts       authMode              authPrincipalSpec
authUrl               authenticator         backlog
caCert                canonicalNaming       clientPrincipal
dateFormat            dateFormatTimeZone    debug
debugMaxEntries       detectorOptions       disableDetectors
disabledServices      discoveryAgentUrl     discoveryEnabled
enabledServices       executor              extendedClientCheck
historyMaxEntries     host                  includeRequest
includeStackTrace     jsr160ProxyAllowedTargets
keyManagerAlgorithm   keyStoreProvider      keystore
keystorePassword      keystoreType          logHandlerClass
logHandlerName        maxCollectionSize     maxDepth
maxObjects            mbeanQualifier        mimeType
multicastGroup        multicastPort         password
policyLocation        port                  protocol
realm                 registerWhiteboardServlet
restrictorClass       serializeException    serializeLong
threadNr              useSslClientAuthentication
user                  useRestrictorService
```

**Total: 58 options**

## Appendix B — Operator-Specific Annotation Keys

These annotations use the `jolokia.horoa.net/` prefix but do NOT correspond to Jolokia agent options. They control operator behavior and MUST be accepted by validation.

| Annotation Key | Purpose | Default |
|---|---|---|
| `mnt` | Mount path for the `jolokia-tmp` emptyDir volume | `/tmp` |
| `rsc-limits-cpu` | Sidecar container CPU limit | *(none)* |
| `rsc-limits-memory` | Sidecar container memory limit | *(none)* |
| `rsc-requests-cpu` | Sidecar container CPU request | *(none)* |
| `rsc-requests-memory` | Sidecar container memory request | *(none)* |
| `target-process` | Target process to attach to — a numeric PID or Java main class name (as returned by Jolokia `list`) | *(auto-detect)* |
| `sidecar-image` | Override sidecar container image reference | `ghcr.io/alxgomz/jolokia-agent:2` (or operator `--sidecar-image` flag) |

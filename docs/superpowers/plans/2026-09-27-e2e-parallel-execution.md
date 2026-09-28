# Safe E2E Parallel Execution Implementation Plan

Implementation note (2026-09-28): the checked-in workflow uses fixed defaults of three core/module workers, two addon workers, one envtest worker and two concurrent matrix jobs. See `test/E2E_PARALLEL.md` for commands, live probe results and remaining full-suite validation. The checklist below records the original planning tasks and is not a claim that every step has been completed.

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Safely parallelize the four requested E2E packages across CI jobs and independent Ginkgo cases while preserving coverage.

**Architecture:** One isolated cluster per live suite job, bounded job fan-out, and Ginkgo process workers within each live suite. Private envtest remains process-local. Shared fixtures use synchronized lifecycle hooks; only global registry-state tests use Serial, and true scenario dependencies use small Ordered groups.

**Tech Stack:** Go 1.23.8 in CI, Ginkgo v2.23.3, Gomega v1.36.2, controller-runtime envtest, KinD Kubernetes v1.31.9, Make, GitHub Actions.

**Spec:** [2026-09-27-e2e-parallel-execution-design.md](../specs/2026-09-27-e2e-parallel-execution-design.md). Read it before executing; it contains evidence, conflict decisions and scope boundaries.

## Global Constraints

- Include exactly the four requested packages; exclude multi-cluster tests and workflows.
- Preserve existing `make e2e-api-test` coverage as an unchanged compatibility lane.
- Preserve pre-existing user changes in `Makefile` and `hack/utils/golangci-lint-wrapper.sh`.
- Local `E2E_PROCS=1`; CI core/module workers=3, addon workers=2, envtest=1, legacy API=1, four-row matrix max-parallel=2. Keep API outside the new suite matrix.
- `E2E_TIMEOUT=1h`; CI per-job timeout=90 minutes. Do not reduce existing assertion timeouts to claim a speedup.
- Ginkgo CLI must match go.mod (currently v2.23.3); no dependency upgrade or production behavior change.
- No goroutine/t.Parallel spec execution, broad namespace cleanup, forced finalizer removal, or shared-current-context mutation.
- Independent fixture names generated only in executing hooks/specs; spec tree and text stable across processes.
- Overlapping complete live-suite invocations require separate clusters and kubeconfig files.
- Preserve 277 discovered specs, 276 active and one existing pending observability spec, unless a reviewed one-to-one scenario mapping explains a count change.

## Review Focus

- A worker fails before setup finishes: cleanup must release only resources actually created, and other workers must not hang (Tasks 1–3, 5).
- Names truncate or restrictions use `tenant-*`: uniqueness survives truncation and namespace matching assertions retain meaning (Tasks 1, 3).
- A controller recreates source cache/owned Applications during cleanup: deletion must wait for producers, then prove owned outputs gone (Tasks 3, 4).
- Default registry selection observes extra entries or a failed serial spec: its baseline must be deterministic and restored (Task 4).
- Matrix cancellation, setup failure or missing reports hides tests: other rows continue and job/report failure remains visible (Tasks 6–7).

## Task 1: Establish discovery baseline and reusable isolation helpers

**Files:** Create `test/e2e-framework/isolation.go`, `test/e2e-framework/isolation_test.go`; create `test/e2e/README.md` only if that directory already exists, otherwise use `test/E2E_PARALLEL.md` (preferred final documentation path). Read all four `suite_test.go` files and the spec before edits. Put validation outputs outside tracked source, under a uniquely named temporary directory.

**Interfaces:** Package `e2eframework` exports `NewRunID() (string, error)`, `UniqueName(prefix string, worker int) (string, error)`, `CreateNamespace(ctx context.Context, cli client.Client, name, runID, suite string, worker int) (*corev1.Namespace, error)`, `DeleteOwnedNamespace(ctx context.Context, cli client.Client, ns *corev1.Namespace, runID string) error`. Use labels `e2e.kubevela.io/run`, `e2e.kubevela.io/suite`, `e2e.kubevela.io/worker`. Keep Ginkgo assertions and cleanup registration at call sites.

- [ ] Capture `git status --short` and `git diff -- Makefile hack/utils/golangci-lint-wrapper.sh`. Do not stage, overwrite or format those unrelated changes.
- [ ] Capture a verbose serial dry-run JSON report before restructuring, using module-matched CLI and all four explicit package paths. Assert the counts in the spec and record leaf spec identities, not just totals. `--dry-run` must use `--procs=1`.
- [ ] Write standard Go tests: `TestUniqueNameDNSAndLength` validates DNS-1123, maximum 63 characters, empty/long/invalid prefixes, preserved `tenant-` prefix; `TestUniqueNameAcrossWorkersAndInvocations` checks 10,000 generated names across workers with independent invocations; `TestCreateNamespaceDoesNotAdoptCollision` requires AlreadyExists to propagate; `TestDeleteOwnedNamespaceRejectsForeignOwner` verifies a foreign namespace remains; `TestDeleteOwnedNamespaceNotFound` succeeds; `TestDeleteOwnedNamespaceUIDChanged` preserves a replacement namespace. Use a fake client/interceptor and explicit UID preconditions where supported; verify errors other than NotFound propagate.
- [ ] Run `go test ./test/e2e-framework -count=1`, observe missing implementation failures, then implement the helpers. `UniqueName` uses a sanitized readable prefix, worker number and at least 12 cryptographically random hexadecimal characters. Truncate only the prefix; never the entropy. `NewRunID` is 16 random hexadecimal characters. CI's run/attempt is report metadata; the launcher generates a fresh token for each invocation.
- [ ] Creation never deletes existing resources or accepts AlreadyExists. Cleanup verifies run label and original UID, then requests deletion with UID precondition. Callers perform bounded termination polling. Register `DeferCleanup` immediately after successful allocation, before the next fallible step.
- [ ] Run helper tests and gofmt only new/changed Go files. Commit this task's scoped files when appropriate for the execution workflow.

## Task 2: Synchronize core suite infrastructure

**Files:** Modify `test/e2e-test/suite_test.go`, `test/e2e-test/auth_registry_helpers_test.go`; create `test/e2e-test/parallel_helpers_test.go`.

**Interfaces:** Keep `k8sClient`/scheme process-local. Add `initializeCoreClient() (client.Client, error)` and a JSON-serializable `coreSuiteState` containing run ID and owned RBAC names. Process-one hook initializes its own client before using it; all-process hook initializes each worker's client and decodes state. Adapt `randomNamespaceName` to Task 1 while preserving its callers until converted.

- [ ] Replace core BeforeSuite/AfterSuite with synchronized counterparts. In the process-one function, create owned RBAC, ensure existing workload definition without claiming ownership, then set up auth registries if enabled. In the all-process function initialize clients/schemes and private command environment. Do not assume globals created in process one are visible in others.
- [ ] Record each successful shared allocation immediately for partial-failure cleanup. The last process-one teardown waits for all workers, removes owned RBAC (including the currently leaked ClusterRole), tears down owned auth registries, and restores only controller fields introduced by auth setup. Preserve pre-existing values for local reruns; require dedicated test-cluster ownership.
- [ ] Keep fixed auth DNS/certificate SANs intact. Wait for rollout/readiness and chart publication before any worker starts Helm auth tests. Capture the fixture state in reports. Do not let a worker's ordinary AfterSuite delete shared resources.
- [ ] Replace `freePort` probe/rebind with portforward `0:<remote>`, use `GetPorts()` after ready, and close/wait on every return path. Use the same explicit rest.Config/KUBECONFIG selection for the client and forwarding.
- [ ] Add a focused lifecycle verification procedure to `test/E2E_PARALLEL.md`: one worker finishes immediately, another retains a registry consumer; confirm registry and binding remain until all finish. Also induce setup failure before and after registry creation and verify no nil-client panic/hung workers. Execute these probes on the disposable cluster in Task 7; do not misrepresent a fake client as proof of process synchronization.
- [ ] Compile/discover core with `go run github.com/onsi/ginkgo/v2/ginkgo --dry-run --procs=1 ./test/e2e-test`; all 237 leaf specs remain registered.

## Task 3: Isolate core cases and fixtures across files

**Files:** Modify `test/e2e-test/application_test.go`, `suite_test.go`, `helmchart_test.go`, `requiredparam_validation_test.go`, `postdispatch_trait_test.go`, `definition_namespace_restriction_test.go`, `definition_test.go`, `app_resourcetracker_test.go`, `config_workflow_test.go`, `source_helpers_test.go`, `source_definition_test.go`, `source_expression_test.go`, `source_builtin_configmap_test.go`. Audit remaining core files' hooks against the design table; change only ownership/namespace handling that fails that audit.

**Interfaces:** `helmTestContext` owns application and release namespaces separately, both allocated in its executing setup hook. Add source helper `sourceDefinitionName(base, namespace string) string` with a bounded base plus a hash of the unique namespace, keeping the result short enough that generated ConfigTemplate names retain the hash. References use the exact returned name. Test helpers may use Task 1; production source identity code remains unchanged.

- [ ] Replace delete-before-create in `createNamespace` with create-only semantics and immediate cleanup registration. Adapt existing cleanup hooks to avoid double deletion. Keep case namespaces independent; ensure the config workflow's namespace is actually deleted, not only its Application.
- [ ] Convert required-parameter tests from outer Ordered/BeforeAll to BeforeEach with one namespace/definition per It. Their three assertions are independent. Keep app fixture DeepCopy behavior.
- [ ] Allocate Helm context names at execution, using strong names and separate application/release namespaces. Preserve deliberate cross-namespace references and namespace deletion/recreation scenarios. Keep each multi-step healing/adoption context Ordered; no blanket Serial or file-level Ordered. Replace the ad-hoc `/tmp/helm-reapply-*` file with CreateTemp and exact cleanup.
- [ ] Move PostDispatch and notification definitions into the owning case namespace. Where system lookup itself is tested (namespace restrictions), retain system placement but suffix definition names and update all expected names/references. Register exact deletion immediately; do not rely on namespace cleanup to delete system definitions.
- [ ] Make the PV-producing component/name unique, use cluster-scoped PV keys, and clean/check that exact PV and owning ResourceTrackers after deleting the Application. Scope any tracker list by both application and namespace identity.
- [ ] Give every test-created SourceDefinition a unique name and update application-, component-, trait-, policy- and workflow-source `Type` references. Do not rename source binding aliases used in expressions unless necessary. Add per-case uniqueness to fixture cache keys, maintaining intentional same-case cache sharing/staleness behavior. Leave chart-installed built-in source definitions immutable.
- [ ] Fix source cleanup ordering: remove/wait for producers and namespaces before cleaning system-generated resources; select only own labels. Cover ConfigTemplate and Config CRs in source-expression cleanup, as well as Secrets and existing legacy ConfigMaps. Assert the owned lists become empty, not just that delete requests succeeded.
- [ ] Add standard unit tests to the framework only for extracted pure naming/fixture code. For source schema collision proof, live-run two cases using identical base source names/schema with different namespaces; assert distinct generated template refs and that cleaning one does not remove the other's template/cache. Keep this as a targeted validation probe or regression spec with an explicit inventory addition.
- [ ] Run gofmt, helper tests and core dry-run inventory comparison. Schedule full randomized two-worker core validation in Task 7; no new retries/skips to conceal collisions.

## Task 4: Break the module-wide Ordered group into isolated cases/scenarios

**Files:** Modify `test/e2e-module-test/suite_test.go`, `helpers_test.go`, `module_e2e_test.go`, `module_publish_test.go`, `testdata/module/registry.yaml`; create `test/e2e-module-test/fixture_helpers_test.go`. Put pure fixture transformation code/tests in `test/e2e-framework/module_fixture.go` and `module_fixture_test.go` so it can be tested without a live suite.

**Interfaces:** Export `CopyModuleFixture(src, dst, oldName, newName, capabilityName string) error` from framework. Add module-local `moduleCase` containing `Name`, `Namespace`, `OtherNamespace`, `FixtureV1`, `FixtureUpgrade`, `RegistryName`, `RegistryURL`, `RegistryBase`, and `TempDir` (all strings). `newModuleCase(ctx context.Context, prefix string) *moduleCase` allocates a unique identity and registers cleanup. Registry suite state holds unique namespace, allocated NodePort, reachable URL and one run-owned immutable entry, broadcast via synchronized hooks.

- [ ] Unit-test fixture copies: both version `_module.cue` identities, derived module references, auxiliary names, RoleBinding roleRef/ServiceAccount subjects, consumer type forms and renderedBy expectations change consistently; versions/API lines and semantic payloads remain intact; source files are unchanged; unrelated text is not rewritten. Give each copy a unique capability name and rewrite Form 1/2 references and definition labels consistently: `pkg/appfile/type_resolution.go` can fall back to a cluster-wide label search, so separate namespaces with identical `bucket` labels still collide. Preserve two API lines with the same unique capability name inside the ambiguity case. Bound module names so `module-<name>-deploy` and derived definitions fit Kubernetes limits. Transform only fixture files/text with known fixture identity tokens; parse copied CUE/YAML with repository parsers as a check.
- [ ] Establish the registry once in synchronized suite setup: namespace from Task 1, dynamic NodePort (omit 30500 in fixture), discover actual port and node InternalIP, validate reachability from the test host and controller path. Broadcast endpoint. Preserve `MODULE_E2E_REGISTRY_URL` for a dedicated external test registry; fail on conflicting ownership instead of replacing an unrelated endpoint.
- [ ] Parameterize `applyManifestFile` to accept an explicit namespace and fixture identity mapping. Do not rewrite Namespace/cluster-scoped objects blindly. Replace fixed `default` consumers and cleanup with case-owned namespaces. Publish unique module artifacts; ordinary parallel cases use the immutable registry entry without registry add/delete operations.
- [ ] Remove outer `Ordered` and its cross-context state. Preserve all leaf assertion names/mapping. Split as follows:

| Existing specs | New scheduling/preconditions |
|---|---|
| Scenario 1 registry management, lines 119–151 | Narrow Serial group. Each It establishes required one/two-entry baseline, verifies behavior, restores baseline and removes own Secrets even on failure. |
| Publish, lines 169–194 | Independent It cases, each with a uniquely named fixture/artifact. Republish/force steps remain sequential within their It. |
| Install/use, lines 213–287 | Independent It cases, each installing its own module at pinned 1.0.0 before assertions. Consumers use the case namespace; where global lookup is the behavior, install unique identities in vela-system. |
| Reconciler, lines 312–349 | Independent It cases with their own pinned installation. Upgrade and unpinned/resync steps remain sequential inside the relevant It. The 80-second resync assertion is preserved. |
| Uninstall, lines 398–440 | Small Ordered three-It scenario with its own upgraded installation and consumer; uninstall → absent-capability behavior → recovery. |
| Namespace, lines 479–534 | Small Ordered four-It ownership scenario with its own module and two private namespaces; no dependency on uninstall scenario. |
| Git refusals, line 560 | Independent negative URL-validation It. |
| Git registry mutation/refusal, lines 594–716 | Narrow Serial cases since they mutate the registry map or require exact registry cardinality; use separate identities and reset each case. Include addon-registry store mutation in this audited group. |
| Error paths, lines 759–785 | Malformed-properties admission is independent and uses a private namespace. Unknown-registry rejection stays in the ordered scenario with a known registry entry as its precondition; moving it out changes the error branch being tested. |
| `module_publish_test.go` lifecycle It | Independent unique widget fixture and namespace; shared immutable registry service/entry. Keep package/publish/immutability/force/deploy/health checks in this It. Registry CLI registration assertions remain covered explicitly by registry cases and synchronized setup; document this step mapping. |

- [ ] Serial groups start only after ordinary workers finish. They must not depend on a particular serial-case order. Snapshot suite-owned registry configuration; establish exact required cardinality for each case; restore on every exit. Do not delete the whole registry ConfigMap or touch unrelated user's entries. Local preflight requires a dedicated test cluster; unexpected foreign state yields an actionable failure.
- [ ] Delete consumers/deploy Applications and wait for nested `vela-system/module-<unique>` Applications, definitions, auxiliary objects, and trackers before removing namespace or shared registry. Avoid the current consumer-v2 leak and ignored errors. Keep shared registry until synchronized teardown after all serial tests.
- [ ] Run `go test ./test/e2e-framework -count=1`, gofmt and module dry-run: 36 active leaf specs retained. Run independent publish + install + reconcile cases together and registry cases alone in Task 7.

## Task 5: Addon and envtest lifecycle boundaries

**Files:** Modify `test/e2e-addon-test/suite_test.go`, `addon_test.go`, `test/e2e-addon-module-test/suite_test.go`, `addon_module_nest_test.go`. No production renderer/controller modifications.

**Interfaces:** Add addon-local CLI helper taking `context.Context` and args, using absolute `bin/vela`, explicit environment and timeout. Synchronized addon setup broadcasts run identity and successful installation state. Envtest keeps its existing per-process cfg/client/singletons and disabled metrics endpoint.

- [ ] Move the two existing active addon enable commands and success-output checks into process-one synchronized setup, sequentially. Wait for both installations and their webhooks/controllers; initialize all clients after the barrier. Retain each workload It and its per-addon readiness/provider checks. Document installation assertions now run in setup. Do not execute or remove the pending observability test.
- [ ] Give both workload It cases independently owned namespaces with Task 1. Update nested namespace fixture fields as well as metadata where used. Remove `/tmp/vela` dependency and deletion of an uninitialized/unowned Application. No worker disables/removes shared addons; job teardown owns their singleton resources.
- [ ] Keep the envtest nesting test as one sequential lifecycle It. Use the namespace helper but do not wait for Kubernetes namespace GC that envtest does not run. Private control-plane shutdown is the final isolation boundary. Register stop cleanup immediately after Start succeeds; cancel and await manager exit before stopping API server, and guard partial setup. Keep feature gates/renderers process-local and restore hooks where supported.
- [ ] Compile/discover both suites: 1 envtest, 2 active addon plus 1 pending. On a disposable live cluster, confirm the two addon workload cases overlap only after both installations complete. Run envtest once and repeat it three times when assets are available.

## Task 6: Bounded launch configuration and CI jobs

**Files:** Modify `makefiles/e2e.mk`, `.github/actions/e2e-test/action.yaml`, `.github/actions/e2e-test/README.md`, `.github/actions/env-setup/action.yaml`, `.github/workflows/e2e-test.yml`; create `hack/e2e/run-suite.sh`, `hack/e2e/run-suite-test.sh`, `test/E2E_PARALLEL.md`. Do not edit the dirty root Makefile or multicluster workflows. Preserve upgrade workflow's existing default composite call.

**Interfaces:** Runner `hack/e2e/run-suite.sh <suite>` accepts only the four exact suite names. Environment: `E2E_PROCS` positive integer default 1; `E2E_TIMEOUT` default 1h; `E2E_REPORT_DIR` default `_artifacts/e2e/<suite>/<run-id>`; `E2E_RUN_ID` generated if absent; optional `E2E_SEED` for repeatable runs. It invokes module-matched `go run github.com/onsi/ginkgo/v2/ginkgo`, accepts no untrusted shell-evaluated flag strings, and uses arrays. Core auth setup remains enabled for full coverage. Existing Make targets call this runner; add the missing `e2e-addon-module-test` target.

- [ ] Write shell contract checks with a stubbed `go` executable: all four suite names map correctly; 0/negative/nonnumeric workers and unknown suite fail; paths with spaces survive; Ginkgo failure returns nonzero despite tee; reports are unique across suite/run IDs; seed and timeout reach Ginkgo. Add invalid-config failure before any cluster mutation.
- [ ] Implement runner with `set -euo pipefail`, `--procs`, `--timeout`, `--trace`, `--json-report`, `--junit-report`, and suite log. Use absolute report paths, stable seed logging, `--fail-on-empty`, and no `--fail-on-pending` (one intentional pending spec exists). Merged reports need one unique path per invocation, not competing identical per-worker report writers.
- [ ] In env-setup install Ginkgo from the checked-out module (`go install github.com/onsi/ginkgo/v2/ginkgo`, without a conflicting `@version`), and log/check its version. This is a shared tool-version correction, not an expansion into multicluster scheduling.
- [ ] Add composite input `suite`, default `all`, plus worker/timeout/report inputs. `all` retains existing API/addon/core/module sequence for upgrade callers; use explicit step-status conditions so independent tests still run after a test failure but not after failed setup. Specific suite inputs run only their selected target. `api` is accepted by the composite to invoke unchanged `make e2e-api-test`, not by the four-suite runner. Envtest preparation installs assets and avoids KinD/Helm/live-cluster setup.
- [ ] Matrix rows: exactly `e2e-test`, `e2e-module-test`, `e2e-addon-test`, and `e2e-addon-module-test`. Keep `api` as a separate unchanged serial compatibility job, gated after the matrix if needed to keep at most two active live-cluster runners. Long core/module rows first is a reasonable queue default, not a scheduling guarantee. Live rows each create one fresh KinD cluster; envtest skips it. Pass actual `v1.31.9` to cluster setup. Keep existing image-load/build/Helm-test/feature-gate preparation for live rows; run setup once per row, never as a background per-suite operation.
- [ ] Configure `strategy.fail-fast: false`, max-parallel=2, live workers=2 by default and envtest/API=1. Expose positive-integer workflow_dispatch/repository-variable overrides using validated inputs. Cancellation group includes workflow/ref/version/suite. Retain no-op gating and job-level failure status. Keep normal PR and upgrade callers working.
- [ ] Report/upload under `always()` (with absent-report diagnostics when setup fails). Unique artifact names contain suite/version/run/attempt. Include JSON, JUnit, suite logs and cluster diagnostics without credential dumps. Keep Codecov enabled when configured and use suite-specific names; do not claim the current `coverage.txt` upload proves E2E coverage. Preserve existing coverage production/collection and surface missing expected outputs.
- [ ] Document local one-worker and two-worker commands, envtest asset prerequisites, isolated-cluster requirement for concurrent packages/runs, unchanged pending case, and per-job resource budget. Run shell contract tests, `bash -n`, available ShellCheck/actionlint and targeted Make dry runs. Review rendered workflow expressions, matrix keys and action compatibility.

## Task 7: Prove coverage, repeatability, cleanup and runtime

**Files:** Update `test/E2E_PARALLEL.md` with measured evidence and limitations. Do not broaden production scope to fix unrelated failures. Keep reports as artifacts, not committed large blobs.

- [ ] Run `go test ./test/e2e-framework -count=1`, targeted formatting checks and compile/discovery commands below. Compare full before/after leaf identities and pending states. Hook changes cannot silently hide assertions behind setup guards.
- [ ] Execute complete serial and two-worker live suites on separate fresh, equivalently prepared clusters. Use explicit kubeconfig paths, same source/images/feature gates/auth configuration and seeds. Run envtest at one worker. Do not use a parallel dry-run: Ginkgo rejects that configuration.
- [ ] Repeat representative parallel groups with seeds 101, 202, 303: core source definitions/expressions + Helm auth + namespace restrictions + PV tracking; module publish/install/reconciler/uninstall/namespace groups; addon workloads. Check distinct namespaces, source templates, nested module Applications and ports. Compare actual JSON timing intervals and ParallelProcess fields to demonstrate overlap, not merely a worker flag.
- [ ] Test ordered groups on one worker with other specs active; test registry Serial groups only after workers drain. Inject one spec failure and one setup failure on a throwaway branch/fixture and verify cleanup, nonzero exit and other matrix rows continuing. Revert the deliberate failure before completion.
- [ ] After each run, query owned namespaces, system definitions, Config/ConfigTemplate/Secrets, nested Applications, ClusterRoles/Bindings, PVs and ResourceTrackers. No run-owned objects should remain except documented shared fixture state awaiting suite teardown. Record finalizer/timeout failures with diagnostics; never force-finalize to make the check green.
- [ ] Execute overlapping workflow runs on separate clusters to verify artifact uniqueness and no shared filesystem/context changes. Same-live-cluster overlapping invocations remain unsupported; explicitly state this boundary.
- [ ] Measure at least three comparable serial/parallel runs when practical. Record suite setup/test/cleanup and complete workflow elapsed times, machine resources and seeds. Report medians and individual runs; distinguish added envtest coverage from the former workflow's work. Do not compare dry-run timings to the user's approximately 50-minute estimate.
- [ ] Review final diff against the scope and user changes. Only report successful commands actually executed. If live infrastructure remains unavailable, list exact remaining commands and do not claim collision safety or speed improvement as measured.

## Validation command sheet

Commands from repository root; those using new Make variables/targets apply **after implementation**. Provision each live suite's dedicated cluster with the existing image-load, e2e-setup-core and Helm-test preparation from the CI action. Do not concurrently prepare several clusters from one checkout because modify_charts.sh edits shared files. Use distinct checkouts for concurrent local suite jobs, with separate KUBECONFIG files.

```bash
# Current and future: compile and inventory without touching a cluster.
go run github.com/onsi/ginkgo/v2/ginkgo --dry-run --procs=1 --no-color --keep-going \
  ./test/e2e-addon-module-test ./test/e2e-addon-test \
  ./test/e2e-module-test ./test/e2e-test

# After implementation: helper/launcher/config checks.
go test ./test/e2e-framework -count=1
bash hack/e2e/run-suite-test.sh
bash -n hack/e2e/run-suite.sh
actionlint .github/workflows/e2e-test.yml
make -n e2e-test E2E_PROCS=2
make -n e2e-module-test E2E_PROCS=2
make -n e2e-addon-test E2E_PROCS=2
make -n e2e-addon-module-test E2E_PROCS=1

# Each live command uses its own previously prepared cluster.
KUBECONFIG=/absolute/path/core.kubeconfig make e2e-test E2E_PROCS=1 E2E_SEED=101 E2E_REPORT_DIR=/tmp/core-serial-101
KUBECONFIG=/absolute/path/core-parallel.kubeconfig make e2e-test E2E_PROCS=2 E2E_SEED=101 E2E_REPORT_DIR=/tmp/core-parallel-101
KUBECONFIG=/absolute/path/module.kubeconfig make e2e-module-test E2E_PROCS=2 E2E_SEED=101 E2E_REPORT_DIR=/tmp/module-parallel-101
KUBECONFIG=/absolute/path/addon.kubeconfig make e2e-addon-test E2E_PROCS=2 E2E_SEED=101 E2E_REPORT_DIR=/tmp/addon-parallel-101

# Existing envtest setup tool/assets; no Docker required if binaries run locally.
KUBEBUILDER_ASSETS="$(./bin/setup-envtest use 1.31.0 --bin-dir ./bin -p path)" \
  make e2e-addon-module-test E2E_PROCS=1 E2E_REPORT_DIR=/tmp/addon-module-101

# Representative core parallel repetition, after prerequisites and CLI env setup.
# Repeat with 202 and 303 and different report paths; do not use dry-run here.
KUBECONFIG=/absolute/path/core-parallel.kubeconfig KUBEVELA_E2E_AUTH=1 \
  go run github.com/onsi/ginkgo/v2/ginkgo --procs=2 --seed=101 --timeout=1h \
  --focus='SourceDefinition e2e|Source expressions across surfaces|Helmchart Auth|Definition namespace restrictions|Test application cross namespace resource' \
  --json-report=/tmp/core-focus-101.json ./test/e2e-test
```

## Research-pass results and handoff

- Completed source/workflow/helper/fixture audit and inspected actual reference implementation at the pinned commit in the spec.
- Serial dry-run compiled all four packages successfully: 277 specs, 276 active, one pending. Wall time 16.137627102s is compilation/discovery only.
- Two-worker dry-run rejected by Ginkgo's serial-only dry-run rule. This attempted check is recorded as failed, not parallel validation.
- Live Kubernetes connection refused; Docker daemon socket unavailable. No live E2E runs, repeated collision tests or speedup measurement performed. The envtest scenario was compiled/discovered but not executed in this planning pass.
- Only design/plan documents are changed by this pass. The next model should read both documents, recheck repository drift, and execute task-by-task using `superpowers:executing-plans` unless the user chooses delegated execution.

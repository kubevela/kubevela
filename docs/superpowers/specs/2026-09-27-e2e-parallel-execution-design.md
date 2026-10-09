# Safe parallel execution of the four E2E packages (superseded)

Status: historical research baseline from September 27, superseded by the rebase onto `guidewire-oss/master` and subsequent implementation. This is not an executable design for the current checkout. Use [E2E suite execution](../../../test/E2E_PARALLEL.md) for current architecture, commands and validation results. All scope, counts, worker budgets, envtest descriptions and measurements below refer to the original research checkout; they are not current coverage or post-rebase validation.

The current workflow has eight matrix suite jobs (four core groups, module, source, addon and addon/module), plus the API/addon-component compatibility job. Matrix jobs run on separate live clusters with CPU-based `auto` worker defaults and no fixed worker cap; the compatibility job keeps its original execution settings. The current addon/module package uses a live cluster and has 82 specs, rather than the one-spec envtest package researched below. The historical discovery timings remain evidence only for the earlier revision.

## Historical scope and intended outcome

Reduce the ordinary E2E workflow's elapsed time while preserving assertions, discovery, failures, diagnostics, and local serial execution. Include exactly these packages in the new parallel execution work:

- `test/e2e-addon-module-test`
- `test/e2e-addon-test`
- `test/e2e-module-test`
- `test/e2e-test`

Exclude multi-cluster tests and workflows. Preserve the existing `make e2e-api-test` coverage as an unchanged compatibility lane; it is not a fifth package to refactor. The user's final instruction requests research and a plan, with implementation in a subsequent lower-model pass.

## Evidence and historical execution model

Research baseline: KubeVela commit `799c7af7e31fdd760386eb15210e7336f51d4c04`. The working tree already has user changes in `Makefile` and `hack/utils/golangci-lint-wrapper.sh`; preserve both. No `AGENTS.md` was found in the repository or checked ancestor locations.

`go.mod:53-54` specifies Ginkgo **v2.23.3**, Gomega **v1.36.2**, and Go **1.23.8**. `.github/actions/env-setup/action.yaml:93` currently installs Ginkgo CLI **v2.14.0**, a mismatch to correct. Each directory has one `Test...` entry point calling `RunSpecs`. Files contribute nodes to a package-wide spec tree; they are not separate suites. Ginkgo schedules individual specs and entire outermost `Ordered` containers among worker processes. `Ordered` alone does not exclude unrelated specs; `Serial` does, within one suite invocation. See [Ginkgo documentation](https://onsi.github.io/ginkgo/#spec-parallelization) and the locally inspected v2.23.3 `internal/ordering.go` and `core_dsl.go`.

`.github/workflows/e2e-test.yml` has one live-test job, with only a Kubernetes-version matrix. `.github/actions/e2e-test/action.yaml` builds and prepares a cluster, then invokes API, addon, core, and module targets sequentially. Module has `if: always()`; earlier failures skip the other later ordinary steps. The composite is also used by `upgrade-e2e-test.yml`, so its default invocation must remain compatible. The workflow matrix says `v1.31` but does not pass that value to cluster setup; the actual setup action default is `v1.31.9`.

`makefiles/e2e.mk:89-106,243-248` invokes Ginkgo without worker flags. Setup installs shared controllers/addons, edits the chart on disk, starts a mock server on port 9098, and copies the CLI to `/tmp/vela`. `e2e-cleanup` deletes the shared Vela home. Do not run these preparation targets concurrently in one checkout/runner.

Discovery measured using the module-matched Ginkgo CLI:

| Package | Discovered | Active | Existing pending | Current useful scheduling units |
|---|---:|---:|---:|---|
| addon-module | 1 | 1 | 0 | One dependent nesting/GC scenario; private envtest |
| addon | 3 | 2 | 1 | Two addon installation/use cases |
| module | 36 | 36 | 0 | One 35-spec outer `Ordered` group plus one publish/deploy spec |
| core | 237 | 237 | 0 | Ordinary specs plus small Helm `Ordered` scenarios and an unnecessarily ordered required-parameter group |

The envtest package is absent from both the current E2E composite and `unit-test-core` package lists. Add an explicit CI lane; do not pretend it was previously executed by these jobs.

## Reference repository: what actually transfers

Inspected a shallow checkout of [kubevela/vela-go-definitions](https://github.com/kubevela/vela-go-definitions) at `8b762d0635bb8649d854a26820f1b8d8e26aed11`:

- [definition_e2e_test.go](https://github.com/kubevela/vela-go-definitions/blob/8b762d0635bb8649d854a26820f1b8d8e26aed11/test/e2e/definition_e2e_test.go) registers a separate `It` per YAML application example and labels four categories. Several applications in one fixture remain sequential inside that `It`.
- [Makefile](https://github.com/kubevela/vela-go-definitions/blob/8b762d0635bb8649d854a26820f1b8d8e26aed11/Makefile) passes `--procs=$(PROCS)` and category label filters, defaulting to **10** workers. Its ordinary aggregate Make prerequisites are not themselves an explicit parallel suite scheduler.
- [test-definitions.yaml](https://github.com/kubevela/vela-go-definitions/blob/8b762d0635bb8649d854a26820f1b8d8e26aed11/.github/workflows/test-definitions.yaml) has four independent jobs, each setting up its own environment.
- [helpers_test.go](https://github.com/kubevela/vela-go-definitions/blob/8b762d0635bb8649d854a26820f1b8d8e26aed11/test/e2e/helpers_test.go) puts applications/prerequisites in a per-example namespace and registers `DeferCleanup`. However, its namespace is deterministic (`e2e-<sanitized-app-name>`), it tolerates AlreadyExists, and its Make targets force-clean broad `e2e` namespace prefixes. These are unsafe patterns for overlapping runs on one cluster and must not be copied.

Adopt independent case registration, process workers, isolated CI environments, parameterized fixtures, and cleanup ownership. Do not adopt the worker count, deterministic names, forced finalizer removal, indiscriminate namespace rewriting of cluster-scoped resources, or broad cleanup.

## Options and decision

1. **Recommended: isolated suite jobs plus Ginkgo workers.** A fresh cluster per live package removes cross-package controller/addon conflicts. Within packages, isolate cases and retain only actual dependency chains. This duplicates some setup but bounds per-cluster load and provides a straightforward safety boundary.
2. **All packages concurrently on one cluster.** Saves setup but conflicts with addon installs, controller CA patches, registry state, and lifecycle cleanup. Ginkgo `Serial` is not a lock across packages. Reject for this change.
3. **One cluster per case.** Strongest isolation but excessive controller/image setup and resource use for 237 core cases. Reserve for a future incompatible global-state test; do not make it the default.

## Scheduling and capacity

Use a matrix with exactly the four requested suite rows. Keep the existing API target as a separate, unchanged serial compatibility job so its coverage is preserved without adding it to the new parallel scope. Leave `max-parallel` unset so new suite rows can run concurrently as runners become available. Core and module use **3** Ginkgo processes, addon uses **2** for its two active specs, and addon-module envtest and legacy API use **1**. The API job runs alongside the matrix on its own runner and cluster. Matrix jobs have separate runners, not multiple KinD clusters sharing a runner. Envtest has only one spec, so extra workers currently buy nothing.

Expose validated `E2E_PROCS` (default **1** locally), `E2E_TIMEOUT` (default **1h**, matching Ginkgo's existing default), `E2E_REPORT_DIR`, and `E2E_RUN_ID`. CI exposes configurable worker and job limits with conservative defaults above, `fail-fast: false`, and a **90-minute** per-job timeout. Log actual CPU/memory and worker settings; these defaults are a starting resource budget, not a measured optimum. Do not copy reference `PROCS=10` or infer throughput from CPU count alone.

Matrix cancellation keys must include suite and Kubernetes version. Otherwise sibling jobs can cancel one another. Keep duplicate-change gating, existing coverage behavior, image preparation, controller feature gates, and upgrade workflow defaults. Reports/artifacts include suite, Kubernetes version, run ID, and attempt. Ginkgo's CLI produces merged JSON/JUnit for each suite; worker-specific logs use process-number suffixes. No package/file regex sharding that silently omits specs.

## Isolation audit and required treatment

| Resource or behavior | Evidence | Required treatment |
|---|---|---|
| Namespace create/delete | `application_test.go:48`, addon helper delete-before-create; many random namespaces already exist | Generate fresh names at runtime; never pre-delete or adopt collisions. Label run/suite/worker ownership. Register cleanup immediately after creation. Preserve `tenant-`/`other-` prefixes used by restriction tests. |
| Definitions | Core helpers and definitions/revisions tests mostly already use local namespaces | Keep namespaced definitions local; await controller/cache readiness. Avoid broad deletes in shared namespaces. |
| System definitions | PostDispatch definitions use random names in `vela-system`; restriction definitions use fixed names; notification definition installed globally | Move PostDispatch and notification definitions to case namespace when lookup semantics permit. Keep restriction tests' system placement but use unique names/references and exact-object cleanup. |
| Source schemas/cache | `sourcedefinition_controller.go:155,213` uses definition name + schema hash, not namespace; `source_cache_identity.go` fingerprints template/properties/referenced context | Give test-created SourceDefinitions unique names and update every source-type reference. Make test-owned cache keys unique as well. Keep uniqueness early enough to survive schema-name truncation. Labels alone do not prevent naming collisions. |
| Source cleanup | Three source test files clean shared-system cache objects; source-expression cleanup still targets ConfigMaps, unlike current Config/ConfigTemplate CR storage | Wait for owning Applications/SourceDefinitions to disappear, then delete/check only own ConfigTemplate, Config, Secret and legacy ConfigMap objects. Use source-definition namespace/context labels. Never delete built-in definitions. |
| RBAC and workload definition | Core `BeforeSuite` creates fixed ClusterRole/Binding and `deployments.apps`; `AfterSuite` deletes binding per worker | Process-one setup behind `SynchronizedBeforeSuite`; unique RBAC names and ownership; shared installed definition immutable; cleanup only resources created by the run after all workers finish. |
| Auth registries/controller CA | Core suite hooks; fixed `kubevela-auth-test`, fixed certificate SANs, CA injection patches Deployment; every current worker would repeat it | Core job owns cluster. One synchronized setup creates registries, injects CA, waits for rollout and publishes chart before releasing workers. Synchronized teardown waits for all workers. Keep certificate/DNS identity together; do not naively rename namespace. Restore owned CA changes on reusable local cluster. |
| Port-forward | `auth_registry_helpers_test.go:589-625` probes and releases a free port before binding | Let portforward bind `0:<remote>` and read `GetPorts()` after readiness; close/wait on all paths. Existing temporary credential files already use CreateTemp. |
| Helm state | Many small `Ordered` contexts; helper generates 4-character names at tree construction, application namespace defaults to `default` | Allocate names in executing hooks. Use separate owned application/release namespaces so target-namespace deletion coverage still works. Retain dependent healing/adoption sequences; convert independent singleton contexts to per-case hooks as appropriate. Use CreateTemp for reapply manifest. |
| Cluster-scoped PV/ResourceTrackers | `app_resourcetracker_test.go:115` creates `pv-cluster-scope-trait-comp`; trackers are cluster-scoped | Unique PV/component name; empty namespace in PV key; exact cleanup after owning app removal. Trackers selected by owning app name AND namespace labels or exact keys; no global counts/deletes. |
| Global policies | `policy_transforms_test.go:145` creates global policy in a test namespace | Already namespace constrained; do not serialize merely because the title says global. Keep scoped assertions. |
| Addon installs | Two active addon cases enable `terraform-alibaba` and `vela-workflow`; mutate system controllers/CRDs/RBAC; one observability `PIt` | Execute both installation commands and success assertions once, sequentially, in synchronized setup; run the two independent workload/provider cases on workers afterward. Retain installation validation in setup and per-case readiness assertions. Pending observability stays pending. Do not add Serial to the entire addon suite. |
| Module registry server | Both module files own/delete `default/oci-registry`, fixed NodePort 30500 | Suite-owned registry in unique namespace, dynamically allocated NodePort, read back Service port; publish reachable endpoint to workers. Preserve explicit URL override, requiring a test-owned endpoint. One setup/teardown owner. |
| Registry records | `pkg/module/registry.go`, `pkg/registry/component/registry.go:255,343` read-modify-write shared `vela-module-registry` without retries | One immutable OCI registration for parallel tests. `Serial` only on registry mutation/cardinality tests. Save/restore suite baseline around those tests on the dedicated test cluster. Unique entry names alone do not fix this race. No production retry/store redesign. |
| Module identity | `module_e2e_test.go` outer Ordered group shares demo-store installation; owned Application is always `vela-system/module-<module>` | Unique module name/fixture copy per independent case or true scenario, including both version fixtures, definition refs, auxiliary names and RBAC subjects. Private tenant/consumer namespaces alone are insufficient. |
| Module short-form capability lookup | `pkg/appfile/type_resolution.go:100-138,144-226` aggregates Form 1/2 matches across the app namespace and `vela-system`, then falls back to a cluster-wide list; `consumer-v1-bucket.yaml` assumes only one module supplies `v1/bucket` | Give parallel module fixtures a unique capability name as well as a unique module name, rewriting `v1/bucket`, bare `bucket`, labels and expected definition names together. Otherwise keep only the two Form 1/2 lookup specs in a narrow Serial group and ensure no other module fixtures remain when it runs. Preserve the deliberate two-API-line ambiguity within its own module. |
| Module dependencies | Install → reconcile → uninstall → tenant ownership currently share state across contexts | Give independent cases their own preconditions. Keep uninstall/recovery and tenant ownership scenarios as small Ordered groups with their own fixtures. No outer Ordered covering all 35 specs. |
| Envtest | Private API server/etcd, process-wide feature gates/singletons/renderers, metrics disabled | Preserve private envtest per Ginkgo process and explicit envtest clients. Never synchronize one envtest server across all workers. Keep the one nesting lifecycle scenario as one It. Stop manager before environment, safely on partial setup failure. |
| Files/process state | `/tmp/vela`, `~/.vela`, chart edits, fixed mock port, kubeconfig, Helm cache | Resolve built CLI by absolute repository path. Per-process VELA_HOME/Helm cache/config paths supplied to subprocesses; preserve real HOME and explicit KUBECONFIG. Shared preparation runs once per isolated job. No `use-context`, Chdir, mutable shared temp files or concurrent chart edits in workers. |

KubeVela definitions are namespaced CRs; their CRDs, RBAC, PVs and ResourceTrackers are cluster-scoped. Do not conflate these scopes. No cluster-registration changes are needed for the four selected packages. Dormant fixtures such as the module `s3` example do not establish executed coverage; do not claim those CRDs are exercised by these 36 specs.

## Cleanup and overlap contract

Independent cases own namespaces with a cryptographic token plus worker identity and readable prefix. A distinct token per invocation also protects resources across overlapping CI runs. Use Kubernetes ownership labels and exact object identity; tolerate only NotFound on deletion, not permission/network errors. Collect diagnostics before deleting failed-case resources, then use bounded cleanup and surface leaks.

Cluster-wide fixtures remain job-owned. Overlapping full live-suite invocations require distinct clusters and kubeconfig files, including locally: auth controller patches, addon singleton names and registry-cardinality tests cannot safely overlap on the same live cluster. `Serial` does not change this limitation. Fresh case namespaces do not make that unsupported mode safe. Never wipe or restore a user's unrelated registry state; require a dedicated test cluster and fail preflight if the suite cannot establish ownership safely.

## Acceptance and validation

1. All 277 specs remain discovered, with 276 active and the same one pending; preserve existing assertions when hooks/scenarios change. Record any name mapping explicitly.
2. Core and module reports show independent cases from the same file and different files running on at least two processes. Ordered dependency chains stay on one process. Addon workload cases can overlap after installation barrier; the sole envtest scenario stays intact.
3. Unit checks prove name bounds/uniqueness, fixture namespace/reference rewrites, and cleanup ownership. Both serial and two-worker complete live runs pass; repeat representative runs three times with different seeds and check exact owned-resource leaks.
4. Suite failure does not cancel other matrix rows. Setup failure fails its row; diagnostics/report upload still run. Teardown waits for all workers, including workers that finish early.
5. Measure before/after workflow elapsed time and suite elapsed time on comparable images, hardware, revisions and cluster setup. Report setup/build costs and failures. No claimed speedup until measured.

Historical research validation: the serial Ginkgo dry run compiled all four packages and discovered the counts above (command wall time 16.137627102s; **not an E2E runtime**). A two-process dry-run attempt was rejected by v2.23.3 because dry runs are serial-only; it does not validate parallel execution. At that time local Go was 1.27.1, different from CI's 1.23.8, the kubeconfig pointed at `k3d-kubevela-pr7367`, whose API endpoint refused connections, and Docker had no available daemon socket. No live E2E run or runtime improvement was measured in that research pass.

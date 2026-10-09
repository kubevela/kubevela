# E2E suite execution

The ordinary E2E workflow runs nine independent suite jobs, each with its own KinD cluster: five core groups plus module, source, addon and addon/module. Every matrix job defaults to one Ginkgo worker per available logical CPU, detected with `nproc` (`getconf _NPROCESSORS_ONLN` is the local fallback). A runner exposing four CPUs gets four workers; one exposing eight gets eight. There is no fixed worker cap. The matrix has no `max-parallel` cap: all current and future rows can run concurrently when GitHub provides runners. The existing API E2E target runs alongside the matrix on another cluster with its original execution settings. Worker counts are resolved separately on each runner; on nine four-CPU runners the matrix would use 36 workers in aggregate. Workflow dispatch uses the same automatic defaults without requiring inputs.

`E2E_PROCS=auto` is also the local Make default. Set `E2E_PROCS=1` for a serial run or any positive integer for a specific worker count. The composite action accepts `auto` or a positive integer in `procs` and `addon-procs`; both default to `auto`. `hack/e2e/ginkgo_workers.sh` resolves and validates the setting for CI and local targets, and each run logs the number passed to Ginkgo. We pass an explicit `--procs` value because Ginkgo's own `-p` detection reserves one CPU when more than four are available.

The core groups are separate Go test packages. Each folder has its own suite entry point and owns the tests for that feature area. Shared client bootstrap, reconciliation helpers, RBAC ownership, Helm support and fixtures live in `test/e2e-framework`. Fixtures are resolved from that package instead of a suite's working directory. Each suite has its own client and scheme in each worker; shared setup runs once through `SynchronizedBeforeSuite`, and teardown waits for every worker through `SynchronizedAfterSuite`. Authentication setup is registered only by the Helm-auth package. The source tests live in `test/e2e-source-test` as a separate package.

```text
test/
├── e2e-application-test/   # Applications, resources and traits
├── e2e-definition-test/    # Definitions and validation
├── e2e-config-test/        # Config management and policies
├── e2e-helm-test/          # Helm lifecycle
├── e2e-helm-auth-test/     # Helm authentication
├── e2e-framework/         # Shared test support
│   └── testdata/          # Shared fixtures, including auth registries
└── e2e-source-test/       # Source definitions and expressions
```

| CI suite / Ginkgo label | Package folder | Specs | Auth registries |
| --- | --- | ---: | --- |
| `core-application` | `test/e2e-application-test` | 54 | No |
| `core-definitions` | `test/e2e-definition-test` | 64 | No |
| `core-config` | `test/e2e-config-test` | 19 | No |
| `core-helm` | `test/e2e-helm-test` | 53 | No |
| `core-helm-auth` | `test/e2e-helm-auth-test` | 17 | Yes |

`bash hack/e2e/verify_core_shards.sh` runs the framework's unit tests and cluster-free Ginkgo discovery for all five packages. It verifies that every package is nonempty, spec names are unique, and each spec has the classification matching its folder. The application CI job runs this check. The counts above describe the current 207 core specs; adding tests to an existing suite does not require updating the checker. `make e2e-test` and the action's `suite: core` run all five packages sequentially on one prepared cluster, with parallel workers inside each package. In the ordinary E2E matrix, each group runs its own package on a separate cluster. The action's `suite: all` runs the five core packages once along with the other test packages.

The addon/module suite's dependent live-cluster scenarios remain one `Ordered` group while independent offline CLI validation and immutable registry-publication checks may run on other workers. Registry publication completes in `SynchronizedBeforeSuite` before any worker starts specs; teardown waits for all workers. Each worker has a private temporary `VELA_HOME` for CLI state. Increasing worker count does not parallelize the dependent steps inside an `Ordered` group.

Ginkgo parallelizes specs and outermost `Ordered` containers within a package, not individual Go files. The module lifecycle remains one `Ordered` scenario because installation, upgrade, uninstall and tenant ownership depend on previous state. The malformed-properties case can run on another worker. The unknown-registry case stays in the ordered scenario: it tests an unknown name while a valid registry is configured. The module publish/deploy case is `Serial` because it updates the shared module-registry ConfigMap. Each module worker uses a private temporary `VELA_HOME`, leaving the kubeconfig untouched. The source-definition and source-expression families now run in their own package and cluster, ordered within each family: generated `vela-system` ConfigTemplate names do not include the source namespace. Built-in ConfigMap source cases have per-case namespaces and can run independently. The pending addon observability case stays pending.

The core component-read family also stays ordered: every case creates `component-read-region`, whose generated system ConfigTemplate name is shared across cases despite their distinct namespaces. Other application specs still run on the remaining workers. Helm scenarios that intentionally adopt, upgrade, delete or recreate a release retain their `Ordered` containers. Splitting CI groups does not break those scenario boundaries.

Selected live-suite jobs build the `vela` CLI with Go's build cache. Module, source, addon/module and both native Helm groups use `e2e-setup-core-minimal` (an alias of `e2e-setup-core-module`), which keeps the same controller chart, feature gates, readiness check and Helm test, but skips the Kruise, Flux and Terraform post-hook that their specs do not use. Other core groups, addon and the API job keep the full environment setup. Only the Helm authentication group brings up the core auth-test registries and injects their CA into the controller. The existing addon-component target runs once in the API job, preserving its prior coverage without repeating it in every matrix job.

Use a dedicated test cluster and kubeconfig for each simultaneous live-suite invocation. Suite-level resources include auth registries, controller trust configuration, addon installations and the module registry ConfigMap. Ginkgo `Serial` applies only inside one package invocation; it is not a lock shared by separate invocations. Prepare the cluster and build the test CLI using the same `setup-kind-cluster` and `e2e-setup-core` steps used by `.github/workflows/e2e-test.yml` before running a live suite locally.

Install the Ginkgo CLI from this module so its version matches `go.mod`:

```bash
go install github.com/onsi/ginkgo/v2/ginkgo
```

Local serial and parallel runs on a prepared, dedicated live cluster:

```bash
KUBECONFIG=/absolute/path/to/test.kubeconfig make e2e-test E2E_PROCS=1
KUBECONFIG=/absolute/path/to/test.kubeconfig make e2e-test E2E_PROCS=2
KUBECONFIG=/absolute/path/to/test.kubeconfig make e2e-core-application-test
KUBECONFIG=/absolute/path/to/test.kubeconfig make e2e-core-definitions-test
KUBECONFIG=/absolute/path/to/test.kubeconfig make e2e-core-config-test
KUBECONFIG=/absolute/path/to/test.kubeconfig make e2e-core-helm-test
KUBECONFIG=/absolute/path/to/test.kubeconfig make e2e-core-helm-auth-test
KUBECONFIG=/absolute/path/to/test.kubeconfig make e2e-module-test E2E_PROCS=1
KUBECONFIG=/absolute/path/to/test.kubeconfig make e2e-module-test
KUBECONFIG=/absolute/path/to/test.kubeconfig make e2e-source-test E2E_PROCS=1
KUBECONFIG=/absolute/path/to/test.kubeconfig make e2e-source-test
KUBECONFIG=/absolute/path/to/test.kubeconfig make e2e-addon-test
KUBECONFIG=/absolute/path/to/test.kubeconfig make e2e-addon-module-test E2E_PROCS=1
KUBECONFIG=/absolute/path/to/test.kubeconfig make e2e-addon-module-test
```

Use `E2E_TIMEOUT=1h` and `E2E_REPORT_DIR=/absolute/path/to/reports` to override the default timeout and report directory. Each target writes a suite-specific JSON and JUnit report; Ginkgo records the process assigned to each spec. Run `ginkgo --dry-run --procs=1` to inspect discovery. Ginkgo v2 does not support parallel dry runs, so a dry run does not verify concurrent behavior.

For a collision check on an available dedicated cluster, run each suite at its detected CPU count at least three times with different `--seed` values, retain its JSON reports and inspect process assignments, failures, namespaces, system definitions, nested Applications, RBAC and ResourceTrackers after teardown. Compare wall-clock time with one-worker runs on equally prepared clusters before claiming a speedup.

Before rebasing onto `guidewire-oss/master`, three focused core runs with two workers passed all three required-parameter specs, and one run with three workers assigned a passing spec to each of processes 1, 2, and 3. The entire 11-spec AutoUpdate family passed locally with three workers after replacing fixed revision sleeps with exact revision readiness checks; the two DependsOn specs that had collided on a fixed namespace passed together with two workers after using per-case namespaces. Exploratory module runs with two workers passed both error-path specs; an audit then found that moving the unknown-registry case had silently changed its configured-registry precondition. That case was restored to the ordered scenario, and the remaining independent malformed-properties case passed live on process 2. Test namespaces were confirmed gone after Kubernetes finished terminating them. The earlier addon/module envtest result does not validate the newer upstream live-cluster suite. The local k3d cluster uses Kubernetes v1.35.5 and has a failed Helm release, unlike CI's v1.31.9 setup. A module publish scenario could not reach the k3d NodePort from the host; this environment requires `MODULE_E2E_REGISTRY_URL` pointing at an endpoint reachable from both the CLI and controller. The live addon suite encountered missing Terraform prerequisites, so its two workload specs did not both pass. A full post-rebase core run and a comparable workflow runtime have not yet been verified.

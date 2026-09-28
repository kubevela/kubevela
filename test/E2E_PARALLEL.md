# E2E suite execution

The ordinary E2E workflow runs `e2e-test`, `e2e-module-test`, `e2e-addon-test`, and `e2e-addon-module-test` as separate jobs. Each live-cluster job has its own KinD cluster. Core and module use three Ginkgo processes; addon has only two active specs and uses two. At most two matrix jobs run at once, so at most six live test workers run across two separate clusters. Workflow dispatch uses these same fixed defaults. Standard public `ubuntu-22.04` runners currently provide four vCPUs and 16 GB RAM; private-repository runners are smaller, so lower the checked-in worker values before using this workflow there. The addon/module nesting suite has one spec and uses one envtest process without KinD. The existing API E2E target runs in a separate serial job after the matrix, preserving its coverage and the two-runner live-cluster limit.

Ginkgo parallelizes specs and outermost `Ordered` containers within a package, not individual Go files. The module lifecycle remains one `Ordered` scenario because installation, upgrade, uninstall and tenant ownership depend on previous state. The malformed-properties case can run on another worker. The unknown-registry case stays in the ordered scenario: it tests an unknown name while a valid registry is configured. The module publish/deploy case is `Serial` because it updates the shared module-registry ConfigMap. Each module worker uses a private temporary `VELA_HOME`, leaving the kubeconfig untouched. Core source-definition and source-expression cases remain ordered within their respective families: generated `vela-system` ConfigTemplate names do not include the source namespace. Other independent core cases can run on other workers. The pending addon observability case stays pending.

Use a dedicated test cluster and kubeconfig for each simultaneous live-suite invocation. Suite-level resources include auth registries, controller trust configuration, addon installations and the module registry ConfigMap. Ginkgo `Serial` applies only inside one package invocation; it is not a lock shared by separate invocations. Prepare the cluster and build the test CLI using the same `setup-kind-cluster` and `e2e-setup-core` steps used by `.github/workflows/e2e-test.yml` before running a live suite locally.

Install the Ginkgo CLI from this module so its version matches `go.mod`:

```bash
go install github.com/onsi/ginkgo/v2/ginkgo
```

Local serial and parallel runs on a prepared, dedicated live cluster:

```bash
KUBECONFIG=/absolute/path/to/test.kubeconfig make e2e-test E2E_PROCS=1
KUBECONFIG=/absolute/path/to/test.kubeconfig make e2e-test E2E_PROCS=2
KUBECONFIG=/absolute/path/to/test.kubeconfig make e2e-module-test E2E_PROCS=1
KUBECONFIG=/absolute/path/to/test.kubeconfig make e2e-module-test E2E_PROCS=2
KUBECONFIG=/absolute/path/to/test.kubeconfig make e2e-addon-test E2E_PROCS=2
```

Run the private envtest suite without Docker (the setup tool and binaries must be installed):

```bash
KUBEBUILDER_ASSETS="$(./bin/setup-envtest use 1.31.0 --bin-dir "$(pwd)/bin" -p path)" make e2e-addon-module-test
```

Use `E2E_TIMEOUT=1h` and `E2E_REPORT_DIR=/absolute/path/to/reports` to override the default timeout and report directory. Each target writes a suite-specific JSON and JUnit report; Ginkgo records the process assigned to each spec. Run `ginkgo --dry-run --procs=1` to inspect discovery. Ginkgo v2 does not support parallel dry runs, so a dry run does not verify concurrent behavior.

For a collision check on an available dedicated cluster, run the core and module suites with two or three workers at least three times with different `--seed` values, retain their JSON reports and inspect process assignments, failures, namespaces, system definitions, nested Applications, RBAC and ResourceTrackers after teardown. Compare wall-clock time with one-worker runs on equally prepared clusters before claiming a speedup.

During local implementation, three focused core runs with two workers passed all three required-parameter specs, and one run with three workers assigned a passing spec to each of processes 1, 2, and 3. Exploratory module runs with two workers passed both error-path specs; an audit then found that moving the unknown-registry case had silently changed its configured-registry precondition. That case was restored to the ordered scenario, and the remaining independent malformed-properties case passed live on process 2. Test namespaces were confirmed gone after Kubernetes finished terminating them. The addon/module envtest scenario passed twice with an absolute `KUBEBUILDER_ASSETS` path. The local k3d cluster uses Kubernetes v1.35.5 and has a failed Helm release, unlike CI's v1.31.9 setup. A module publish scenario could not reach the k3d NodePort from the host; this environment requires `MODULE_E2E_REGISTRY_URL` pointing at an endpoint reachable from both the CLI and controller. The live addon suite encountered missing Terraform prerequisites, so its two workload specs did not both pass. Full core/module/addon suites, repeated full-suite collision checks, and a comparable workflow runtime were therefore not verified locally.

# e2e-addon-module-test

Live-cluster e2e suite for addons installed as `type: addon` components that
import modules through `modules/_imports.cue`. It is the automated form of the
scenarios in `testing/addon-module-cr-based-single-cluster` and follows the
shape of `test/e2e-module-test`.

## What the cluster needs

- vela-core built from this source, installed with
  `featureGates.enableAddonComponent=true`,
  `featureGates.enableModuleComponent=true`,
  `admissionWebhooks.enabled=true` and `controllerArgs.reSyncPeriod=1m`
  (`make e2e-test-local` does all of this).
- `bin/vela` built from this source (`CGO_ENABLED=0 go build -o bin/vela ./references/cmd/cli`).
- The images `docker.io/library/registry:2` and
  `ghcr.io/helm/chartmuseum:v0.16.2` loadable by the cluster.

The suite brings up its own registries from `testdata/registry.yaml`:

| registry | used for | why |
| --- | --- | --- |
| `registry:2`, NodePort 30500 | modules | the same Deployment/Service `test/e2e-module-test` uses |
| ChartMuseum, NodePort 30501 | addons | the controller reads an OCI addon registry over TLS only, so a plain-HTTP registry can only serve addons as a Helm repository |

Each registry record stores one URL, `http://<node InternalIP>:<port>`, and
the controller pod always reaches that. The test process itself may not: on
a Mac the k3d node sits on a docker network the host cannot route to. The
suite therefore probes the node-IP URL from the test process and, when it
does not answer, uses the k3d port mapping `http://127.0.0.1:<port>` for its
own publishes, pushes and HTTP checks (`make e2e-local-cluster` creates the
cluster with `-p 30500:30500@server:0 -p 30501:30501@server:0`). On a Linux
runner the node IP is routable, so one URL serves both sides.

Two specs need the vela CLI on the test machine to fetch *through* the
stored record: `vela module deploy --dry-run` (scenario 01) and `vela addon
enable` (scenario 16). They are skipped, with the reason printed, when the
node-IP URL is unreachable from the host, and run on a Linux runner.

Overrides, all optional:

| variable | meaning |
| --- | --- |
| `MODULE_E2E_REGISTRY_URL` | URL stored in the module registry record (default `http://<node IP>:30500/modules`) |
| `ADDON_E2E_REGISTRY_URL` | URL stored in the addon registry record (default `http://<node IP>:30501`) |
| `MODULE_E2E_REGISTRY_HOST_URL` | URL this process uses for modules when the stored one is unreachable (default `http://127.0.0.1:30500/modules`) |
| `ADDON_E2E_REGISTRY_HOST_URL` | same for addons (default `http://127.0.0.1:30501`) |

To run every spec on a Mac, including `test/e2e-module-test`, give both
sides one name: add `127.0.0.1 host.k3d.internal` to `/etc/hosts` (k3d
already resolves that name to the host inside the cluster) and export
`MODULE_E2E_REGISTRY_URL=http://host.k3d.internal:30500/modules` and
`ADDON_E2E_REGISTRY_URL=http://host.k3d.internal:30501`.

## Running

```bash
make e2e-addon-module-test-local    # k3d cluster + image build + this suite only
make e2e-test-local                 # the same setup, then this suite and test/e2e-module-test
make e2e-addon-module-test          # against an already prepared cluster
```

CI runs one job with one Ginkgo worker per available CPU. Scenarios with
dependent steps use separate `Ordered` containers (08 and 07 share one chain);
the read-only scenario 01 is not ordered. Each scenario has a namespace, module/addon
identities, exported definition aliases, CRD API group and ClusterRole names
specific to that scenario and run. The original fixture files remain intact;
synchronized setup materializes private copies and publishes initial versions
before tests can render. Each worker has its own `VELA_HOME`.

Scenarios 08 and 07 stay together because the first publishes versions the
second uses. Scenarios 05, 09, 15 and 18 use `Serial, Ordered`: they change
default-registry resolution, restart the shared controller or toggle gates.
This leaves 69 specs in parallel-capable scenarios and 13 serial specs. The
same observation windows and parameter/version/line contracts are retained.

Use `E2E_PROCS=1` for serial execution or `E2E_PROCS=auto` (the default)
for parallel workers. `E2E_REPORT_DIR` selects the JSON/JUnit output directory.
For example:

```bash
KUBECONFIG=/absolute/path/to/test-cluster.kubeconfig make e2e-addon-module-test E2E_PROCS=2
make e2e-addon-module-discovery  # no live cluster needed
```

Use a dedicated cluster per complete invocation; separate invocations cannot
coordinate their controller restarts through Ginkgo's `Serial` decorator.
Avoid focusing only scenario 07, since it needs scenario 08 to publish its
module versions. The discovery target verifies fixture parsing, definition
resolution, cleanup ownership, real Ginkgo scheduling metadata and complete
spec discovery. The single addon/module CI job runs it automatically.

After building `bin/vela` and preparing the dedicated cluster, compare
`make e2e-addon-module-test E2E_PROCS=1` with `E2E_PROCS=2` and the default
`E2E_PROCS=auto`. Repeat the parallel run with distinct seeds and keep reports:

```bash
set -euo pipefail
mkdir -p _artifacts/e2e
workers=$(bash hack/e2e/ginkgo_workers.sh auto)
for seed in 101 202 303; do
  ginkgo -v --procs="$workers" --seed="$seed" --timeout=1h --fail-on-empty \
    --json-report="_artifacts/e2e/addon-module-$seed.json" \
    --junit-report="_artifacts/e2e/addon-module-$seed.xml" ./test/e2e-addon-module-test
done
```

Inspect process assignments and timestamps for overlapping independent
scenarios, absence of overlap during the serial phase, all 82 specs passing,
and no scenario namespaces, CRDs, definitions, roles or Applications left
after cleanup. A dry run proves discovery and scheduling configuration, not
live concurrent execution or a runtime improvement.

## Layout

| path | content |
| --- | --- |
| `testdata/modules/` | `widget-kit` 1.0.0/1.1.0/1.2.0, `gadget-kit`, `probe-kit` builds a and b, and `invalid/` trees the publisher must refuse |
| `testdata/addons/` | `widget-platform` 1.0.0/1.1.0/1.2.0, `kit-suite` 1.0.0/2.0.0, `widget-latest`, `import-options`, `tenant-widgets`, `cache-probe` builds a and b, `broken-imports` 1.0.1 to 1.0.7 |
| `testdata/apps/` | consumer Applications that use the module definitions |
| `publish_validation_test.go` | CLI-only checks (`vela module publish --dry-run`) |
| `addon_module_e2e_test.go` | independent ordered scenarios and the narrow serial phase |
| `scenario_scope_helpers_test.go` | generated fixture identities and structured YAML/CUE materialization |
| `scenario_runtime_helpers_test.go` | scenario ownership, cleanup and publication helpers |
| `scenario_scope_test.go`, `scheduling_test.go` | cluster-free isolation and execution-contract tests |

The fixtures are copies of `testing/addon-module-cr-based-single-cluster/test-*`
with the registry names changed to `e2e-modules` and `e2e-addons`.

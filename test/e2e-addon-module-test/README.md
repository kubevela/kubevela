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

The suite is `Ordered`. It publishes the fixtures once, then runs one
`Context` per scenario; every scenario removes what it installed. Two
scenarios restart vela-core and one toggles its feature gates, so do not run
it against a cluster that other tests share.

## Layout

| path | content |
| --- | --- |
| `testdata/modules/` | `widget-kit` 1.0.0/1.1.0/1.2.0, `gadget-kit`, `probe-kit` builds a and b, and `invalid/` trees the publisher must refuse |
| `testdata/addons/` | `widget-platform` 1.0.0/1.1.0/1.2.0, `kit-suite` 1.0.0/2.0.0, `widget-latest`, `import-options`, `tenant-widgets`, `cache-probe` builds a and b, `broken-imports` 1.0.1 to 1.0.7 |
| `testdata/apps/` | consumer Applications that use the module definitions |
| `publish_validation_test.go` | CLI-only checks (`vela module publish --dry-run`) |
| `addon_module_e2e_test.go` | the ordered cluster scenarios |

The fixtures are copies of `testing/addon-module-cr-based-single-cluster/test-*`
with the registry names changed to `e2e-modules` and `e2e-addons`.

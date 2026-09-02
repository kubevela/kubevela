# Running Tests

KubeVela has three broad layers of tests: unit tests (no cluster needed),
controller/API e2e tests (need a live cluster), and a coverage-instrumented
"main e2e" run. This guide covers the `make` targets for each and the
gotchas that trip people up.

## Quick reference

| Goal | Command |
|---|---|
| Unit tests | `make test` |
| Controller e2e (self-provisions a cluster) | `make e2e-test-local` |
| Controller e2e against a cluster you already deployed to | `make e2e-test` |
| One e2e spec / flake check | `ginkgo -v --focus="<text>" ./test/e2e-test` |
| CLI-driven application e2e | `make e2e-application-test-local` |
| Coverage-instrumented main e2e | `make e2e-test-main-local` |
| Addon / multicluster / API e2e | `make e2e-addon-test`, `make e2e-multicluster-test`, `make e2e-api-test` |

## Prerequisites

- Run everything from the repository root.
- If Go tooling reports "no Go files" or similar despite files clearly being
  present, your Git checkout may be flagged as having "dubious ownership"
  (common on bind-mounted/shared filesystems). Fix with:

  ```bash
  git config --global --add safe.directory "$(pwd)"
  ```

- Make sure your `KUBECONFIG` actually points at a reachable, valid
  kubeconfig file before running anything that touches a cluster. An invalid
  or stale `KUBECONFIG` can cause client-go's config loading to fail in ways
  that don't always produce an obvious error message mid-test-run.

## Unit tests

```bash
make test
```

This chains three targets:

1. `envtest`: downloads the pinned Kubernetes control-plane test binaries
   (currently **1.31.0**) via `setup-envtest`, so unit tests can talk to a
   real (in-memory) API server without a full cluster.
2. `unit-test-core`: runs `go test` over `./pkg/... ./cmd/... ./apis/...`
   and `./references/...` (excluding `apiserver` and
   `applicationconfiguration`), using `KUBEBUILDER_ASSETS` from step 1.
3. `test-cli-gen`: regenerates and tests the CLI docs.

`make test` stops at the first failing stage. To run the equivalent
`unit-test-core` command standalone (useful when iterating, or to see every
package's result instead of stopping early):

```bash
KUBEBUILDER_ASSETS="$(bin/setup-envtest use 1.31.0 -p path)" \
  go test $(go list ./pkg/... ./cmd/... ./apis/... | grep -v apiserver | grep -v applicationconfiguration)
```

Scope to one package while iterating on a specific change:

```bash
go test ./pkg/<subpath>/... -count=1
```

> **A few unit test packages depend on external network access or a
> Linux Docker daemon** (registry-auth tests, and the OpenAPI-generator-based
> SDK codegen tests). If you're running in a sandboxed or offline
> environment, or via Docker-outside-of-Docker where bind-mounted paths
> don't line up with the host, expect those specific packages to fail for
> environmental reasons. They pass in CI, where those conditions don't
> apply.

## Controller e2e tests

### Self-provisioned cluster (`make e2e-test-local`)

The most self-contained option. It creates (or reuses) a `kubevela-debug` k3d
cluster, builds and imports a `vela-core:e2e-test` image, pre-loads the
public registry images the "Helmchart Auth" suite needs
(`ghcr.io/project-zot/zot-minimal-linux-amd64`, `ghcr.io/helm/chartmuseum`,
`docker.io/library/nginx`), installs the chart with the webhook enabled, and
runs `./test/e2e-test`:

```bash
make e2e-test-local
```

If cluster creation succeeds but the Helm install can't reach the cluster's
API server, see the networking troubleshooting note in
[`k3d-workflow.md`](./k3d-workflow.md#troubleshooting). The same k3d
reachability issue that affects manual setup can affect this target too.

> **Known gap**: `e2e-test-local`'s Helm install only enables the
> `enableCueValidation` and `validateResourcesExist` feature gates. The
> "Application Policy Transform" specs additionally require
> `EnableApplicationScopedPolicies` and `EnableGlobalPolicies` (both alpha,
> off by default). Without them, those specs will time out waiting for
> behavior that never happens. To run them, add both flags to the Helm
> install (`--set featureGates.enableApplicationScopedPolicies=true --set
> featureGates.enableGlobalPolicies=true`), or patch an already-running
> deployment:
>
> ```bash
> kubectl -n vela-system patch deploy kubevela-vela-core --type=json -p='[
>   {"op":"add","path":"/spec/template/spec/containers/0/args/-","value":"--feature-gates=EnableApplicationScopedPolicies=true"},
>   {"op":"add","path":"/spec/template/spec/containers/0/args/-","value":"--feature-gates=EnableGlobalPolicies=true"}
> ]'
> kubectl -n vela-system rollout status deploy/kubevela-vela-core --timeout=90s
> ```

### Against an already-deployed cluster (`make e2e-test`)

If you've already installed vela-core yourself (e.g. via
[`k3d-workflow.md`](./k3d-workflow.md) or `make e2e-setup-core`), skip the
provisioning and just run the suite:

```bash
make e2e-test
```

### Running a single spec or checking for a flake

Ginkgo's `--focus` is a regex matched against the full `Describe > Context >
It` path. Focus on the parent `Describe` if the spec you care about depends
on a sibling setup spec; focusing too narrowly will skip it.

```bash
ginkgo -v --focus="<some spec description>" ./test/e2e-test

# Run it a few times in a row to rule out a timing-sensitive flake:
ginkgo -v --repeat=2 --focus="<some spec description>" ./test/e2e-test
```

A handful of controller e2e specs are known to be timing-sensitive under CPU
load (they assert on a resource reaching some state without a generous
retry/backoff). If a spec fails once but passes reliably in isolation, that's
more likely a flake than a regression. It's worth confirming against a fresh
cluster before spending time chasing it as a real bug.

## Application e2e tests (`e2e/application`)

This is a CLI-driven suite: the test harness execs the repository's
`bin/vela` binary directly, so that binary's OS/architecture must match
whatever machine the tests run on.

```bash
GO111MODULE=on CGO_ENABLED=0 go build -o bin/vela ./references/cmd/cli/main.go
```

> If you built `bin/vela` on a different OS/architecture than where the
> tests run (e.g. built on macOS, running the tests inside a Linux
> container), the harness will fail immediately with something like
> `exec format error`. Rebuild `bin/vela` for the target platform first.

```bash
make e2e-application-test-local   # self-provisions a cluster, runs the suite, deletes the cluster afterward
```

To run just the test phase against a cluster you already have vela-core
running on:

```bash
# Clean up leftover apps/environments from a previous run first
vela ls -n default --quiet 2>/dev/null | tail -n +2 | awk '{print $1}' | xargs -I {} vela delete {} -n default -y 2>/dev/null || true
vela env delete env-application 2>/dev/null || true
ginkgo -v -r e2e/application
```

> **Reusing a cluster across runs can break the interactive `vela init`
> spec.** It types `webservice` at the workload-type prompt and expects the
> next prompt to offer a `ComponentDefinition` named exactly that. If a
> previous test run left behind a definition whose name merely *contains*
> "webservice" (e.g. one created by an earlier controller e2e run), fuzzy
> matching can select the wrong one and the scripted prompts desync,
> eventually timing out. If you hit this, either start from a fresh cluster
> or delete the stray definition and re-run.

## Coverage-instrumented main e2e (`make e2e-test-main-local`)

This target is not a pass/fail suite. It compiles `cmd/core`'s `main()` as a
test binary, deploys it into its own `kubevela-e2e-main` k3d cluster (with
webhooks and multicluster disabled for a clean install), and lets it run so
code coverage can be collected:

```bash
make e2e-test-main-local
```

Success looks like a `1/1 Running` controller pod that's reconciling
definitions without panics. Check with:

```bash
kubectl logs -n vela-system -l app.kubernetes.io/name=vela-core -f
```

Coverage output is written to `/workspace/data/e2e-profile.out` inside the
pod; the target attempts to copy it out to `./e2e-main-coverage.out`. Clean
up afterward with:

```bash
make e2e-test-main-clean
```

## Other e2e targets

| Target | What it does |
|---|---|
| `make e2e-api-test` | Runs the general API suite plus `e2e/application`, against an already-deployed cluster. |
| `make e2e-addon-test` | Copies `bin/vela` to `/tmp/`, runs `./test/e2e-addon-test`. |
| `make e2e-multicluster-test` | Runs the multicluster suite under `test/e2e-multicluster-test` with its own coverage profile. |
| `make e2e-setup-core` / `make e2e-setup-core-auth` | Installs vela-core into the current cluster (without / with auth + sharding enabled) so you can then run `make e2e-test` against it directly. |
| `make e2e-cleanup` | Removes `~/.vela` local CLI state. |

## Cleanup

```bash
helm uninstall kubevela -n vela-system   # drop vela-core, keep the cluster
make e2e-test-main-clean                  # or: k3d cluster delete kubevela-e2e-main
k3d cluster delete kubevela-debug
```

## Related

- [`k3d-workflow.md`](./k3d-workflow.md): manual cluster setup and the
  devcontainer/container-based networking caveat referenced above.
- [`webhook-debugging.md`](./webhook-debugging.md): debugging the admission
  webhook specifically, if an e2e failure points there.

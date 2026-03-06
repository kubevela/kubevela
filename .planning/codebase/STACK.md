# Technology Stack

## Languages

- **Go 1.23.8** — primary implementation language (all controller, CLI, webhook, and pkg code)
- **CUE v0.14.1** — definition templating language for ComponentDefinitions, TraitDefinitions, WorkflowStepDefinitions; templates live in `vela-templates/definitions/` and `pkg/workflow/template/static/`
- **Bash** — build scripts, CI helpers, entrypoint (`entrypoint.sh`), webhook debug tools
- **YAML/HCL** — Helm chart values, Kubernetes manifests, Terraform config inspection via `github.com/oam-dev/terraform-config-inspect`

## Runtime

- **Kubernetes v1.31.x** — target cluster platform; controller tested against `ENVTEST_K8S_VERSION = 1.31.0` (`makefiles/dependency.mk`)
- **Go binary** — single statically linked `manager` binary built from `cmd/core/main.go`, packaged in `alpine:3.18` Docker image (`Dockerfile`)
- **CLI binary** — `vela` CLI built from `references/cmd/cli/main.go`; kubectl plugin `kubectl-vela` from `cmd/plugin/main.go`

## Core Frameworks & Libraries

### Kubernetes Controller
- `sigs.k8s.io/controller-runtime v0.19.7` — manager, reconciler, webhook, cache, metrics server
- `k8s.io/api v0.31.10`, `k8s.io/apimachinery v0.31.10`, `k8s.io/client-go v0.31.10`
- `k8s.io/apiextensions-apiserver v0.31.10` — CRD management
- `k8s.io/apiserver v0.31.10` — feature gates, impersonation
- `sigs.k8s.io/controller-tools v0.16.5` — code generation (`make generate`, `make manifests`)

### OAM / KubeVela Ecosystem
- `github.com/kubevela/pkg v1.9.3` — shared controller utilities, multicluster client, sharding, metrics
- `github.com/kubevela/workflow v0.6.3` — workflow engine, CUE model/value evaluation
- `github.com/oam-dev/cluster-gateway v1.9.2` — multicluster API gateway
- `github.com/oam-dev/cluster-register v1.0.4` — cluster registration
- `github.com/oam-dev/terraform-controller v0.8.1` — Terraform provider integration
- `github.com/crossplane/crossplane-runtime v1.16.0` — resource claim/composition primitives

### CUE & Configuration
- `cuelang.org/go v0.14.1` — CUE evaluation, schema validation, formatting
- `github.com/hashicorp/hcl/v2 v2.18.0` — HCL config parsing for Terraform definitions
- `github.com/hashicorp/go-version v1.6.0` — semver for addon versioning

### Helm
- `helm.sh/helm/v3 v3.14.4` — chart loading, rendering, install/upgrade (`pkg/utils/helm/`)
- `k8s.io/helm v2.17.0` (legacy v2 support)
- `github.com/chartmuseum/helm-push v0.10.4` — push charts to ChartMuseum
- `sigs.k8s.io/kustomize/api v0.17.2` — kustomize overlays

### CLI
- `github.com/spf13/cobra v1.9.1` — CLI command framework
- `github.com/spf13/pflag v1.0.7` — flag parsing
- `github.com/AlecAivazis/survey/v2 v2.1.1` — interactive prompts
- `github.com/rivo/tview v0.0.0-20221128` — TUI components
- `github.com/gdamore/tcell/v2 v2.6.0` — terminal cell library
- `github.com/briandowns/spinner v1.23.0` — CLI spinners
- `github.com/fatih/color v1.18.0` — colored output

### Observability
- `github.com/prometheus/client_golang v1.20.5` — metrics exposition; custom histograms/gauges in `pkg/monitor/metrics/`
- `go.opentelemetry.io/otel v1.28.0` + OTLP exporter — tracing (gRPC transport via `otlptracegrpc`)
- `go.uber.org/zap v1.26.0` — structured logging
- `k8s.io/klog/v2 v2.130.1` — Kubernetes-style logging

### Serialization & Data
- `gopkg.in/yaml.v3`, `sigs.k8s.io/yaml v1.4.0`, `go.yaml.in/yaml/v3` — YAML processing
- `github.com/tidwall/gjson v1.14.4` — JSON path queries
- `github.com/getkin/kin-openapi v0.131.0` — OpenAPI 3 schema validation
- `gomodules.xyz/jsonpatch/v2 v2.4.0` — JSON patch for admission webhooks
- `github.com/evanphx/json-patch/v5 v5.9.0` — strategic merge patch

### Networking & HTTP
- `github.com/go-resty/resty/v2 v2.8.0` — HTTP client (addon registry, OSS, Gitee API)
- `github.com/gorilla/mux v1.8.1` — HTTP routing
- `golang.org/x/oauth2 v0.30.0` — OAuth2 token source for GitHub/GitLab addon auth
- `google.golang.org/grpc v1.67.1` — gRPC (etcd, cluster gateway, OTel)

### Testing
- `github.com/onsi/ginkgo/v2 v2.23.3` — BDD test framework for E2E and integration tests
- `github.com/onsi/gomega v1.36.2` — matcher library
- `github.com/stretchr/testify v1.10.0` — unit test assertions
- `github.com/golang/mock v1.6.0` — mock generation
- `sigs.k8s.io/controller-runtime/tools/setup-envtest` — envtest K8s API server binary (`makefiles/dependency.mk`)
- `sigs.k8s.io/kind v0.20.0` — local cluster provisioning in CI

## Build Tooling

- **Make** — primary build system; modular makefiles in `makefiles/` (const, build, dependency, develop, release, e2e)
- **Docker** — multi-stage builds; `golang:1.23.8-alpine` builder, `alpine:3.18` runtime (`Dockerfile`, `Dockerfile.cli`, `Dockerfile.e2e`)
- **golangci-lint v1.60.1** — linting (`makefiles/dependency.mk`)
- **staticcheck v0.6.1** — static analysis
- **goimports** — import ordering with local prefix `github.com/oam-dev/kubevela`
- **cue CLI v0.14.1** — CUE formatting and validation for definition templates
- **kustomize v4.5.4** — CRD/manifest overlays
- **controller-gen** via `sigs.k8s.io/controller-tools` — CRD generation (`make generate`, `make manifests`)
- **gox** (`github.com/mitchellh/gox`) — cross-compilation for darwin/amd64, linux/amd64, windows/amd64

## Configuration

- **Helm chart** — primary deployment config at `charts/vela-core/values.yaml`; key parameters: `concurrentReconciles`, `applicationRevisionLimit`, `featureGates.*`, `optimize.*`, `workflow.*`
- **Feature gates** — runtime toggled via `k8s.io/apiserver/pkg/util/feature`; defined in `pkg/features/controller_features.go`
- **Controller flags** — structured config objects per domain in `cmd/core/app/config/` (admission, application, client, multicluster, observability, performance, reconcile, sharding, webhook, workflow)
- **Env vars** — `KUBEBUILDER_ASSETS` for envtest; `GOPROXY` for module proxy; `RUNTIME_CLUSTER_CONFIG` for multicluster worker kubeconfig

## CI/CD

- **GitHub Actions** — workflows in `.github/workflows/`; key workflows: `e2e-test.yml`, `e2e-multicluster-test.yml`, `unit-test.yml`, `release.yml`, `trivy-scan.yml`, `codeql-analysis.yml`
- **KinD** — E2E clusters provisioned via `.github/actions/setup-kind-cluster`; K8s v1.31 matrix
- **Docker Hub** — images pushed to `oamdev/vela-core` and `oamdev/vela-cli`
- **setup-envtest** — unit/integration test K8s binaries downloaded via `makefiles/dependency.mk`

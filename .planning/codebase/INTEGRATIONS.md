# External Integrations

## Multicluster — Cluster Gateway

- **Library**: `github.com/oam-dev/cluster-gateway v1.9.2` (`go.mod`)
- **Purpose**: Routes kubectl/controller API requests to remote managed clusters via an aggregated API server. Enabled with `--enable-cluster-gateway` flag (`cmd/core/app/config/multicluster.go`).
- **Registration**: Managed clusters registered via `github.com/oam-dev/cluster-register v1.0.4`
- **Usage**: `pkg/multicluster/` — `cluster_management.go`, `virtual_cluster.go`; workflow providers in `pkg/workflow/providers/multicluster/`
- **Open Cluster Management (OCM)**: `open-cluster-management.io/api v0.11.0` — OCM API types for ManagedCluster resources

## Git Providers — Addon Registry Sources

### GitHub
- **Library**: `github.com/google/go-github/v32 v32.1.0`
- **Auth**: OAuth2 token via `golang.org/x/oauth2`
- **Usage**: `pkg/addon/reader_github.go`, `pkg/addon/addon.go` — reads addon metadata and files from GitHub repositories
- **API**: GitHub REST API v3; anonymous or token-authenticated

### GitLab
- **Library**: `gitlab.com/gitlab-org/api/client-go v0.127.0`
- **Auth**: Personal access token via `golang.org/x/oauth2`
- **Usage**: `pkg/addon/reader_gitlab.go`, `pkg/addon/source.go` — reads addon files from GitLab project repositories

### Gitee (Chinese Git host)
- **Library**: Custom HTTP client using `github.com/go-resty/resty/v2`
- **Usage**: `pkg/addon/reader_gitee.go` — reads addon content from Gitee API
- **API**: Gitee REST API with custom `Client` struct in `pkg/addon/reader_gitee.go`

### Generic Git (go-git)
- **Library**: `github.com/go-git/go-git/v5 v5.16.0`
- **Usage**: Clone and read addon repositories; SSH agent support via `github.com/xanzy/ssh-agent v0.3.3`

## Object Storage — Alibaba Cloud OSS

- **Library**: HTTP via `github.com/go-resty/resty/v2`; SDK `github.com/aliyun/alibaba-cloud-sdk-go v1.61.1704` (indirect)
- **Usage**: `pkg/addon/reader_oss.go` — lists and reads addon files from OSS buckets using XML bucket listing API
- **URL pattern**: `{scheme}://{bucket}.{endpoint}/{path}` (see `source.go` `bucketTmpl`)

## Helm Chart Registries

- **ChartMuseum**: `github.com/chartmuseum/helm-push v0.10.4` — push charts to ChartMuseum registries (`pkg/addon/push.go`)
- **OCI Registries**: `github.com/google/go-containerregistry v0.18.0`, `cuelabs.dev/go/oci/ociregistry` — OCI artifact push/pull for Helm charts and CUE packages
- **Helm Hub/Repos**: `helm.sh/helm/v3` HTTP repo client for pulling index.yaml and chart tarballs

## Configuration Management — Nacos

- **Library**: `github.com/nacos-group/nacos-sdk-go/v2 v2.2.2`
- **Usage**: `pkg/config/writer/nacos.go` — writes KubeVela config values to Nacos configuration center as a config writer backend
- **Connection**: Via `NacosConfig.Endpoint` referencing a Kubernetes Secret with server addr/credentials

## Infrastructure Providers

### Terraform
- **Library**: `github.com/oam-dev/terraform-controller v0.8.1`, `github.com/oam-dev/terraform-config-inspect v0.0.0-20250902`
- **Usage**: `pkg/utils/common/common.go` — parses Terraform module schemas for ComponentDefinition generation; Terraform resources managed as KubeVela components via terraform-controller CRDs
- **HCL**: `github.com/hashicorp/hcl/v2 v2.18.0` — parses `.tf` files for variable extraction

### Crossplane
- **Library**: `github.com/crossplane/crossplane-runtime v1.16.0`
- **Usage**: `pkg/resourcekeeper/statekeep.go`, `pkg/controller/core.oam.dev/v1beta1/application/apply.go` — resource management primitives; Crossplane XRs/Claims can be managed as KubeVela workloads

## GitOps — FluxCD

- **Libraries**: `github.com/fluxcd/helm-controller/api v0.32.2`, `github.com/fluxcd/source-controller/api v0.30.0`
- **Usage**: `pkg/appfile/helm/flux2apis/helmrelease_types.go` — Flux HelmRelease type definitions; KubeVela can manage Flux HelmRelease objects as workloads
- **API types**: `fluxcd/pkg/apis/acl`, `fluxcd/pkg/apis/kustomize`, `fluxcd/pkg/apis/meta`

## Progressive Delivery — OpenKruise

- **Libraries**: `github.com/openkruise/kruise-api v1.4.0`, `github.com/openkruise/rollouts v0.3.0`
- **Usage**: `pkg/rollout/rollout.go` — rollout status tracking and canary/blue-green deployment integration via Kruise Rollout CRDs

## Kubernetes API Extensions

### Gateway API
- **Library**: `sigs.k8s.io/gateway-api v0.7.1`
- **Usage**: Gateway API resources can be managed as KubeVela workload/trait targets

### Kube Aggregator
- **Library**: `k8s.io/kube-aggregator v0.31.10`
- **Usage**: `cmd/core/app/server.go` — registers cluster-gateway as an aggregated API server for multicluster routing

### Metrics API
- **Library**: `k8s.io/metrics v0.31.10`
- **Usage**: `pkg/multicluster/cluster_metrics_management.go` — collects CPU/memory metrics from managed clusters

## Observability Backends

### Prometheus
- **Library**: `github.com/prometheus/client_golang v1.20.5`
- **Exposition**: Metrics served at `:8080/metrics` (configurable via `--metrics-addr`; `cmd/core/app/config/observability.go`)
- **Metrics**: Application reconcile duration, workflow step phase, cluster capacity gauges — defined in `pkg/monitor/metrics/`
- **Integration**: controller-runtime's built-in metrics server via `metricsserver.Options`

### OpenTelemetry (OTLP)
- **Libraries**: `go.opentelemetry.io/otel v1.28.0`, `otlptracegrpc` exporter
- **Usage**: Distributed tracing exported via gRPC to any OTLP-compatible backend (Jaeger, Tempo, etc.)
- **Instrumentation**: gRPC (`otelgrpc`) and HTTP (`otelhttp`) auto-instrumentation

## Authentication & Authorization

### Kubernetes RBAC / Impersonation
- `pkg/auth/round_trippers.go` — `impersonatingRoundTripper` injects Kubernetes impersonation headers for per-application RBAC enforcement
- `pkg/auth/kubeconfig.go` — generates kubeconfigs backed by Kubernetes ServiceAccounts for scoped access
- `pkg/auth/identity.go`, `userinfo.go` — extracts user identity from Application annotations (`oam.AnnotationApplicationServiceAccountName`)

### JWT (Legacy)
- **Library**: `github.com/form3tech-oss/jwt-go v3.2.5`
- **Usage**: Token validation in older auth paths (indirect dependency)

### OAuth2
- **Library**: `golang.org/x/oauth2 v0.30.0`
- **Usage**: Token sources for GitHub and GitLab addon registry access (`pkg/addon/addon.go`)

## Distributed Key-Value — etcd

- **Libraries**: `go.etcd.io/etcd/client/v3 v3.5.16`, `go.etcd.io/etcd/api/v3`
- **Usage**: Indirect dependency via `k8s.io/apiserver`; used by embedded API server components and leader election

## Container Registries (OCI)

- **Library**: `github.com/google/go-containerregistry v0.18.0`
- **Usage**: Push/pull addon packages and CUE definitions as OCI artifacts; registry interactions in `pkg/addon/`
- **ORAS**: `oras.land/oras-go v1.2.5` — OCI Registry As Storage for addon distribution

## Webhooks (Inbound — Kubernetes Admission)

- **Framework**: `sigs.k8s.io/controller-runtime/pkg/webhook`
- **Port**: 9443 (configurable via `charts/vela-core/values.yaml` `webhookService.port`)
- **Handlers**:
  - Mutating: `pkg/webhook/core.oam.dev/v1beta1/application/mutating_handler.go`, `pkg/webhook/core.oam.dev/v1beta1/componentdefinition/mutating_handler.go`
  - Validating: `pkg/webhook/core.oam.dev/v1beta1/application/validating_handler.go`, TraitDefinition, PolicyDefinition, WorkflowStepDefinition validators
- **Registration**: `pkg/webhook/core.oam.dev/register.go`

## Email Notifications

- **Library**: `gopkg.in/gomail.v2 v2.0.0` (indirect via workflow notification steps)
- **Usage**: Workflow notification steps can send email alerts on application status changes

# Running KubeVela Locally

This guide covers the day-to-day workflow for building, running, and
debugging the KubeVela controller (`vela-core`) on your own machine. It
supplements [`CONTRIBUTING.md`](../../CONTRIBUTING.md) and the
[full contributor guide](https://kubevela.io/docs/contributor/code-contribute)
with concrete, in-repo steps.

## Prerequisites

| Tool | Version | Check |
|---|---|---|
| Go | matching `go.mod` (currently 1.23.x) | `go version` |
| Docker | any recent version | `docker info` |
| [k3d](https://k3d.io/) | v5+ | `k3d version` |
| kubectl | 1.28+ | `kubectl version --client` |
| Helm | v3 | `helm version --short` |
| openssl | any recent version | `openssl version` |
| [Delve](https://github.com/go-delve/delve) (optional, only for CLI-based debugging) | a release supporting Go 1.23+ | `dlv version` |
| VS Code + [Go extension](https://marketplace.visualstudio.com/items?itemName=golang.Go), **or** IntelliJ IDEA / GoLand with the Go plugin | latest | n/a |

## Guides in this folder

Roughly in the order you'd reach for them, from first getting something
running to debugging specific pieces of it:

| Guide | Covers | Reach for it when |
|---|---|---|
| [`ide-debugging.md`](./ide-debugging.md) | Running the controller from your IDE, against any cluster your kubeconfig can reach | You want the fastest inner loop: breakpoints, no image build |
| [`k3d-workflow.md`](./k3d-workflow.md) | Building the real controller image and running it in a local k3d cluster (plus a `ttl.sh`-push alternative) | You want to test the image the way it actually ships, with a fast rebuild/reload loop |
| [`remote-cluster-deployment.md`](./remote-cluster-deployment.md) | Deploying to a real remote cluster (EKS as the worked example, but generic) | You need production-like conditions a local single-node cluster can't reproduce |
| [`ide-remote-cluster-debugging.md`](./ide-remote-cluster-debugging.md) | Attaching Delve to a manager process already running in a pod (local or remote) | You're chasing a bug that only shows up under real cluster conditions |
| [`ide-multi-cluster-debugging.md`](./ide-multi-cluster-debugging.md) | A master/slave k3d cluster pair plus Cluster Gateway, with the controller running from your IDE | You're debugging multi-cluster scheduling/dispatch code, e.g. the `topology` policy |
| [`webhook-debugging.md`](./webhook-debugging.md) | Running and debugging the admission webhook (validating/mutating handlers) locally | You're touching `ComponentDefinition`/`Application` validation or defaulting logic |
| [`testing.md`](./testing.md) | The unit and e2e `make` targets | You're running or adding to the test suites |
| [`logging.md`](./logging.md) | Verbosity and log-format flags | You need more detail out of a running controller |

## Repository layout (development-relevant paths)

```
cmd/core/main.go                 # controller entrypoint
charts/vela-core/                # Helm chart that installs the controller
pkg/                             # controller + provider source
pkg/webhook/core.oam.dev/...     # admission webhook handlers
test/e2e-test/                   # Ginkgo e2e suite (needs a live cluster)
hack/debug-webhook-setup.sh      # generates local webhook TLS certs + config
Makefile, makefiles/*.mk         # build/test/debug targets (see each guide)
```

## Prerequisite reading

If you're new to writing or debugging a Kubernetes controller, these cover
the background KubeVela's controller code assumes:

- [KubeVela Docs](https://kubevela.io/docs/)
- [Kubernetes API Concepts](https://kubernetes.io/docs/reference/using-api/api-concepts/#api-verbs)
- [Extending Kubernetes (Custom Resources)](https://kubernetes.io/docs/tasks/extend-kubernetes/custom-resources/custom-resource-definitions/)
- [Kubernetes API Extension](https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/)
- [KubeBuilder](https://book.kubebuilder.io/)
- [K8s Controller Runtime](https://github.com/kubernetes-sigs/controller-runtime)
- [Working with K8s Objects](https://kubernetes.io/docs/concepts/overview/working-with-objects/)
- [K8s API Machinery](https://github.com/kubernetes/apimachinery)
- [Kubernetes Controllers at Scale: Clients, Caches, Conflicts, Patches Explained](https://medium.com/@timebertt/kubernetes-controllers-at-scale-clients-caches-conflicts-patches-explained-aa0f7a8b4332)

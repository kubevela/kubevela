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

## Three ways to run KubeVela

| Approach | What it tests | When to use |
|---|---|---|
| [IDE debugger](./ide-debugging.md) | The binary, running on your host | Fastest inner loop: step through code with breakpoints, no image build. Needs a reachable cluster (local k3d or remote) with CRDs installed. |
| [Local k3d cluster](./k3d-workflow.md) | The real container image | You want to test the image the way it will actually ship, with a fast rebuild-and-reload loop. |
| [Remote cluster (e.g. EKS)](./remote-cluster-deployment.md) | The image under production-like conditions | Validating behavior against real etcd, real node counts, or provider-specific quirks that a local cluster can't reproduce. |

Once you have something running, see [`logging.md`](./logging.md) for
verbosity/log options, [`testing.md`](./testing.md) for running the unit and
e2e test suites, [`webhook-debugging.md`](./webhook-debugging.md) for the
admission-webhook-specific workflow (definition/application validation), and
[`remote-cluster-debugging.md`](./remote-cluster-debugging.md) for attaching
an IDE debugger to a process already running in a cluster (local or remote).

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

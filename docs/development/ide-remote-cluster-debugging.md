# Debugging a Process Running In-Cluster

The other guides either run the binary on your host
([`ide-debugging.md`](./ide-debugging.md)) or build an image and load it into
a cluster without attaching a debugger
([`k3d-workflow.md`](./k3d-workflow.md), [`remote-cluster-deployment.md`](./remote-cluster-deployment.md)).
This guide covers a third option: attaching [Delve](https://github.com/go-delve/delve)
directly to the manager process already running inside a pod, on a cluster
you don't want to (or can't) run the binary outside of. Use this when you
need to reproduce a bug that only shows up under real cluster conditions, or
you're debugging on a cluster where running the binary locally isn't
practical. It works the same way whether that cluster is a local k3d cluster
or a remote one like EKS.

## Prerequisites

Docker with buildx, kubectl, Helm v3, Go, and push/deploy access to whichever
cluster you're targeting.

## 1. Build a debug-enabled image

The default build strips debug symbols and disables Delve's ability to set
accurate breakpoints. Use a separate `Dockerfile.debug` rather than editing
the repo's real `Dockerfile`, the same way [`k3d-workflow.md`](./k3d-workflow.md)
uses a separate `Dockerfile.local`:

```dockerfile
# Dockerfile.debug
FROM golang:1.23.8-alpine AS builder
WORKDIR /workspace
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/core/ cmd/core/
COPY apis/ apis/
COPY pkg/ pkg/
COPY version/ version/
COPY references/ references/

ARG TARGETARCH
ARG VERSION
ARG GITVERSION
# -gcflags disables optimizations/inlining so breakpoints land on the right
# line. Dropping -s -w (present in the real Dockerfile) keeps debug symbols.
RUN GO111MODULE=on CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} \
    go build -gcflags="all=-N -l" -a \
    -ldflags "-X github.com/oam-dev/kubevela/version.VelaVersion=${VERSION:-undefined} -X github.com/oam-dev/kubevela/version.GitRevision=${GITVERSION:-undefined}" \
    -o manager-${TARGETARCH} cmd/core/main.go

# Match this to your Go toolchain (go.mod) - Delve releases track Go versions.
RUN go install github.com/go-delve/delve/cmd/dlv@latest

FROM alpine:3.18
RUN apk add --no-cache ca-certificates bash expat
WORKDIR /
ARG TARGETARCH
COPY --from=builder /workspace/manager-${TARGETARCH} /usr/local/bin/manager
COPY --from=builder /go/bin/dlv /usr/local/bin/dlv
COPY entrypoint.sh /usr/local/bin/
EXPOSE 40000
ENTRYPOINT ["entrypoint.sh"]
CMD ["manager"]
```

```bash
docker buildx build --platform linux/<arch> -t <your-registry>/vela-core:debug --push \
  --build-arg VERSION=debug --build-arg GITVERSION=debug -f Dockerfile.debug .
```

(See [`remote-cluster-deployment.md`](./remote-cluster-deployment.md) for
detecting the target architecture and registry-login mechanics; they're the
same here.)

## 2. Disable leader election and health probes for this deployment

Breakpoints pause the process, sometimes for a long time. If leader election
or the readiness/liveness probes are active, Kubernetes will fail them and
restart the pod mid-debug-session. `charts/vela-core/templates/kubevela-controller.yaml`
hardcodes `--enable-leader-election` with no `values.yaml` flag to disable
it, so temporarily edit the chart the same way `hack/e2e/modify_charts.sh`
already does for the same reason (see [`testing.md`](./testing.md#coverage-instrumented-main-e2e-make-e2e-test-main-local)):

```bash
cp charts/vela-core/templates/kubevela-controller.yaml{,.bak}
trap 'mv charts/vela-core/templates/kubevela-controller.yaml.bak charts/vela-core/templates/kubevela-controller.yaml' EXIT

# Remove the "--enable-leader-election" line and the readinessProbe/livenessProbe
# blocks from charts/vela-core/templates/kubevela-controller.yaml before continuing.
```

## 3. Deploy the debug image

```bash
helm upgrade --install vela-core ./charts/vela-core \
  --namespace vela-system --create-namespace \
  --set image.repository="<your-registry>/vela-core" \
  --set image.tag=debug \
  --set image.pullPolicy=Always \
  --set securityContext.capabilities.add[0]=SYS_PTRACE \
  --wait --timeout 5m
```

`securityContext` is `{}` by default (`values.yaml`), so without
`SYS_PTRACE` the container has no `ptrace` access and `dlv attach` in step 4
fails immediately with a permission error, before your IDE ever gets a
chance to connect.

> If your cluster enforces a restricted Pod Security Standard (or your own
> `securityContext`/`podSecurityContext` customization drops capabilities),
> it may reject `SYS_PTRACE` outright regardless of this flag. That's a
> cluster-policy problem, not something this chart setting can work around.

## 4. Attach Delve to the running pod

The container's entrypoint runs `manager` as PID 1 (`exec "$@"` in
`entrypoint.sh`, so there's no wrapper process to work around):

```bash
kubectl exec -it <pod-name> -n vela-system -- dlv attach 1 --headless --listen=:40000 --api-version=2 --accept-multiclient
```

Expect:

```
API server listening at: [::]:40000
```

## 5. Forward the debugger port

In a separate terminal (this blocks, so it needs its own session):

```bash
kubectl port-forward pod/<pod-name> -n vela-system 40000:40000
```

## 6. Connect from your IDE

**VS Code** (`.vscode/launch.json`):

```json
{
    "name": "Attach to In-Cluster Pod",
    "type": "go",
    "request": "attach",
    "mode": "remote",
    "port": 40000,
    "substitutePath": [
        { "from": "${workspaceFolder}", "to": "/workspace" }
    ]
}
```

The image was built with `WORKDIR /workspace` (see the Dockerfile in step
1), so the binary's embedded source paths start with `/workspace/...`, not
your local checkout's path. Without `substitutePath` mapping the two, VS
Code can't match the debugger's paths to your local files and breakpoints
won't bind.

**IntelliJ IDEA / GoLand**: Run → Edit Configurations → + → **Go Remote** →
Host `localhost`, Port `40000` → Apply → Debug.

## 7. Trigger a breakpoint

Set a breakpoint (e.g. in a `validating_handler.go`), then apply a matching
resource:

```bash
kubectl apply -f your-componentdefinition.yaml
```

> **Don't edit the source file while attached.** The running binary was
> built from a specific snapshot of the code; editing it locally afterward
> doesn't change what's running in the pod; it just desyncs line numbers
> between your editor and the debugger, making breakpoints land in the wrong
> place.

## Cleanup

```bash
helm uninstall vela-core -n vela-system
mv charts/vela-core/templates/kubevela-controller.yaml.bak charts/vela-core/templates/kubevela-controller.yaml
```

Restore the chart file explicitly here rather than relying on the step 2
trap alone. The trap only fires when the shell itself exits, so if you keep
this terminal open and `helm install` something else in the meantime, it
would silently pick up the debug-only chart (no leader election, no
probes) instead of the real one.

## References

- [Debugging a Go application inside a Docker container (JetBrains)](https://blog.jetbrains.com/go/2020/05/06/debugging-a-go-application-inside-a-docker-container/)
- [Attach to running Go processes with the debugger (JetBrains)](https://www.jetbrains.com/help/go/attach-to-running-go-processes-with-debugger.html)
- [Delve installation docs](https://github.com/go-delve/delve/tree/master/Documentation/installation)

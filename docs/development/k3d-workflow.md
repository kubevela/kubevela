# Building and Running in a Local k3d Cluster

This workflow builds the controller into a real container image and runs it
inside a local [k3d](https://k3d.io/) cluster, which is closer to production
than [IDE debugging](./ide-debugging.md) and much faster to iterate on than a
remote cluster.

## Prerequisites

k3d, Docker, kubectl, Helm v3, Go.

## 1. Create the cluster

```bash
k3d cluster create vela-dev --wait
kubectl config use-context k3d-vela-dev
```

The repo also ships a `make k3d-create` target that does the same thing
(1 server + 1 agent, idempotent, so it's a no-op if the cluster already
exists) against a fixed cluster name, `kubevela-debug`:

```bash
make k3d-create      # creates/reuses "kubevela-debug" and switches kubectl context to it
make k3d-delete      # deletes it
```

This isn't specific to webhook debugging. It's also used as a building block
by [`webhook-debugging.md`](./webhook-debugging.md), and it's just a
convenience for "give me *a* k3d cluster" when you don't care about the name
or node count. Use whichever fits: your own `k3d cluster create <name>` for
full control, or `make k3d-create` when the defaults are fine.

> **Pick one and stick with it.** Steps 4-7 below use the literal name
> `vela-dev` (`k3d image import ... -c vela-dev`, `k3d cluster delete
> vela-dev`). If you use `make k3d-create` instead, its cluster is named
> `kubevela-debug`, not `vela-dev`, so substitute that name in every later
> command, or override it once: `make k3d-create K3D_CLUSTER_NAME=vela-dev`.

## 2. Build the manager binary

Cross-compile for Linux even if you're on macOS or Windows, since the image
runs on the cluster's Linux nodes:

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=$(go env GOARCH) go build \
  -ldflags "-s -w -X github.com/oam-dev/kubevela/version.VelaVersion=dev-$(git rev-parse --short HEAD)" \
  -o bin/manager cmd/core/main.go
```

## 3. Build a local image

A minimal Alpine-based image works well for local iteration:

```dockerfile
# Dockerfile.local
FROM alpine:3.18
RUN apk add --no-cache ca-certificates bash expat
WORKDIR /
COPY bin/manager /usr/local/bin/manager
COPY entrypoint.sh /usr/local/bin/
ENTRYPOINT ["entrypoint.sh"]
CMD ["manager"]
```

```bash
docker build -t vela-core:local -f Dockerfile.local .
```

## 4. Load the image into the cluster

k3d clusters can't pull local-only images from a registry, so import them
directly:

```bash
k3d image import vela-core:local -c vela-dev
```

## 5. Install the chart

```bash
helm install vela-core ./charts/vela-core \
  --namespace vela-system --create-namespace \
  --set image.repository=vela-core \
  --set image.tag=local \
  --set image.pullPolicy=Never \
  --set controllerArgs.reSyncPeriod=1m \
  --wait --timeout 3m
```

`pullPolicy=Never` is what makes the kubelet use the image you just imported
instead of trying to pull it from a registry.

## 6. Verify

```bash
kubectl get pods -n vela-system
kubectl get componentdefinitions.core.oam.dev -n vela-system
```

## 7. Iterate after code changes

Repeat the build/import steps, then force the pod to restart so it picks up
the new image (`pullPolicy=Never` means a plain rollout restart won't
re-pull, so delete the pod instead):

```bash
# Rebuild binary + image (steps 2-3), then:
k3d image import vela-core:local -c vela-dev
kubectl delete pod -n vela-system -l app.kubernetes.io/name=vela-core --force --grace-period=0
kubectl rollout status deployment vela-core -n vela-system --timeout=60s
```

This does **not** touch the cluster or the Helm release, only the image and
the running pod, so it's fast (roughly 30-90s depending on build time).

## Running tests against this cluster

```bash
# Unit tests don't need a live cluster:
go test ./pkg/... -count=1

# e2e tests do:
go test ./test/e2e-test/ -v -count=1 -timeout=30m -ginkgo.focus=Helmchart
```

## Alternative: push to ttl.sh instead of importing

Steps 2-4 skip a registry entirely by importing straight into k3d's
containerd. That's the fastest loop, but it doesn't exercise the actual pull
path (`imagePullPolicy: Always`, a registry the cluster has to reach), and it
doesn't give you an image reference you could hand to someone else. For
those cases, push to [ttl.sh](https://ttl.sh) instead, an anonymous registry
that needs no login and expires images automatically:

```bash
IMAGE_REF="ttl.sh/vela-core-$(git rev-parse --short HEAD):1h"   # expires in 1h

# Build with the real multi-stage Dockerfile (not Dockerfile.local), since
# there's no local binary to COPY in this flow:
docker build \
  --build-arg VERSION="$(git rev-parse --abbrev-ref HEAD)" \
  --build-arg GITVERSION="$(git rev-parse HEAD)" \
  -t "$IMAGE_REF" -f Dockerfile .

docker push "$IMAGE_REF"

helm upgrade --install vela-core ./charts/vela-core \
  --namespace vela-system --create-namespace \
  --set image.repository="${IMAGE_REF%:*}" \
  --set image.tag="${IMAGE_REF##*:}" \
  --set image.pullPolicy=Always \
  --wait --timeout 6m
```

`pullPolicy=Always` means a `kubectl rollout restart deployment vela-core -n
vela-system` after pushing an updated image is enough to pick up the change,
unlike step 7's `Never`-policy flow, which needs the pod deleted outright.

> Don't reach for `k3d cluster delete --all` if you're scripting this
> end-to-end. It deletes every k3d cluster on the host, including unrelated
> ones, for example the master/slave pair from
> [`ide-multi-cluster-debugging.md`](./ide-multi-cluster-debugging.md).
> Delete only the cluster this workflow created (`k3d cluster delete
> vela-dev`), the same as the teardown step below.

## Tearing down

```bash
helm uninstall vela-core -n vela-system   # keep the cluster, drop KubeVela
k3d cluster delete vela-dev               # delete the cluster entirely
```

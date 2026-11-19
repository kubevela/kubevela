# Deploying to a Remote Cluster

Once local testing looks good, deploy to a real cluster to validate behavior
a local single-node k3d cluster won't reproduce, like real etcd behavior,
multiple nodes, or provider-specific networking and IAM. The pattern below is
generic; **Amazon EKS** is used as the worked example since it's what this
repo's tooling currently automates, but the same steps apply to any managed
Kubernetes offering (GKE, AKS, self-managed) by swapping the registry-login
and cluster-auth commands.

To interactively debug a process already running on a remote (or local)
cluster once it's deployed, see
[`ide-remote-cluster-debugging.md`](./ide-remote-cluster-debugging.md).

## Prerequisites

Docker with buildx, kubectl, Helm v3, Go, `jq`, and your cloud provider's CLI
(e.g. `aws`, `gcloud`, `az`) authenticated with push access to a container
registry and admin access to the target cluster.

## 1. Detect the target node architecture

Build for the cluster's actual node architecture, not your local machine's:

```bash
NODE_ARCH=$(kubectl get nodes -o jsonpath='{.items[0].status.nodeInfo.architecture}')
```

## 2. Cross-compile

```bash
CGO_ENABLED=0 GOOS=linux GOARCH="$NODE_ARCH" go build \
  -ldflags "-s -w -X github.com/oam-dev/kubevela/version.VelaVersion=dev-$(git rev-parse --short HEAD)" \
  -o "bin/manager-$NODE_ARCH" cmd/core/main.go
```

## 3. Build a platform-explicit image

```bash
docker buildx build --platform "linux/$NODE_ARCH" \
  -t "<your-registry>/vela-core:<tag>" --load \
  -f - . <<EOF
FROM ${NODE_ARCH}/alpine:3.18
RUN apk add --no-cache ca-certificates bash expat
WORKDIR /
COPY bin/manager-${NODE_ARCH} /usr/local/bin/manager
COPY entrypoint.sh /usr/local/bin/
ENTRYPOINT ["entrypoint.sh"]
CMD ["manager"]
EOF
```

## 4. Push to a registry your cluster can pull from

The login step is provider-specific; everything after it is the same:

```bash
# Amazon ECR:
aws ecr get-login-password --region <region> | docker login --username AWS --password-stdin <registry-host>

# Google Artifact Registry / GCR:
gcloud auth configure-docker <region>-docker.pkg.dev

# Azure Container Registry:
az acr login --name <registry-name>

# Docker Hub / GHCR / self-hosted:
docker login <registry-host>
```

```bash
docker push "<your-registry>/vela-core:<tag>"
```

## 5. Deploy

**Fresh install:**

```bash
helm install vela-core ./charts/vela-core \
  --namespace vela-system --create-namespace \
  --set image.repository="<your-registry>/vela-core" \
  --set image.tag="<tag>" \
  --set image.pullPolicy=Always \
  --set controllerArgs.reSyncPeriod=1m \
  --wait --timeout 5m
```

**Update an existing install** (same image name/tag with `pullPolicy=Always`
in the deployment, so a rolling restart is enough after a fresh push):

```bash
kubectl rollout restart deployment vela-core -n vela-system
kubectl rollout status deployment vela-core -n vela-system --timeout=120s
```

## Useful follow-up operations

**Change the reconcile interval without a redeploy:**

```bash
kubectl get deployment vela-core -n vela-system -o json \
  | jq '(.spec.template.spec.containers[0].args[] | select(startswith("--application-re-sync-period"))) = "--application-re-sync-period=5m"' \
  | kubectl apply -f -
```

**Increase webhook timeouts** (useful for large chart installs where
admission review can otherwise time out):

```bash
kubectl patch validatingwebhookconfigurations vela-core-admission \
  --type=json -p='[{"op": "replace", "path": "/webhooks/0/timeoutSeconds", "value": 30}]'
# repeat for each /webhooks/<index>/timeoutSeconds entry, or patch all at once
# with a full replace of the "webhooks" array if you prefer.
```

## Tearing down

```bash
helm uninstall vela-core -n vela-system
kubectl delete namespace vela-system
```

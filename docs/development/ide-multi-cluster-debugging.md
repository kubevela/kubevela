# Debugging a Multi-Cluster Setup from Your IDE

Debugging against a single cluster from your IDE is covered in
[`ide-debugging.md`](./ide-debugging.md). This one's for the multi-cluster
case: a "master" cluster running [Cluster Gateway](https://github.com/kubevela/cluster-gateway)
in-cluster while the controller itself runs from your IDE, plus a "slave"
cluster joined to it. That's enough to set breakpoints in multi-cluster
scheduling and dispatch code (the `topology` policy, for instance) without
giving up the ability to run the binary on your host.

## Prerequisites

k3d, kubectl, Helm v3, Go, and the `vela` CLI built from this repo
(`make vela-cli`, which puts it at `bin/vela`; the steps below assume it's on
your `PATH` or invoked as `./bin/vela`).

## 1. Create the master cluster

```bash
k3d cluster create master --wait
k3d kubeconfig get master > ~/.kube/master.yaml
export KUBECONFIG=~/.kube/master.yaml
```

Keeping each cluster's kubeconfig in its own file (rather than merging into
your default `~/.kube/config`) makes it easy to point specific tools
(your IDE, `vela cluster join`) at a specific cluster without juggling
contexts.

## 2. Install CRDs and definitions

```bash
make core-install   # applies CRDs from charts/vela-core/crds/
make def-install     # installs default ComponentDefinitions/TraitDefinitions
```

## 3. Install only Cluster Gateway in the master cluster

The controller is going to run from your IDE, so you don't want the chart's
own controller `Deployment` also running in-cluster; both would reconcile the
same resources. Unlike the admission webhook, which the chart already lets
you skip with `--set admissionWebhooks.enabled=false`, there's no values flag
for the controller `Deployment` in `charts/vela-core/templates/kubevela-controller.yaml`,
it's unconditional. Temporarily remove it the same way
[`ide-remote-cluster-debugging.md`](./ide-remote-cluster-debugging.md#2-disable-leader-election-and-health-probes-for-this-deployment)
edits the same file:

```bash
cp charts/vela-core/templates/kubevela-controller.yaml{,.bak}
trap 'mv charts/vela-core/templates/kubevela-controller.yaml.bak charts/vela-core/templates/kubevela-controller.yaml' EXIT

# Remove the "Deployment" block from
# charts/vela-core/templates/kubevela-controller.yaml before continuing.
# Leave the ServiceAccount/ClusterRole/Role/RoleBinding resources above it
# alone, the controller running in your IDE still needs that RBAC.
```

The `Service`/`ServiceMonitor` resources further down the same file are
already gated behind `core.metrics.enabled` (default `false`), so they won't
render unless you've turned metrics on.

```bash
helm install kubevela ./charts/vela-core \
  --namespace vela-system --create-namespace \
  --set admissionWebhooks.enabled=false \
  --set devLogs=true \
  --wait --debug
```

`multicluster.enabled` defaults to `true`, so this installs Cluster Gateway
without any extra flag. Verify:

```bash
kubectl get all -n vela-system
```

You should see only a `kubevela-cluster-gateway` pod/deployment/service, no
`kubevela-vela-core` pod.

## 4. Point your IDE at the master cluster and run the controller

Use the "Run KubeVela Core" configuration from
[`ide-debugging.md`](./ide-debugging.md#vs-code), adding (or confirming) a
`KUBECONFIG` environment variable pointing at `~/.kube/master.yaml`:

```json
"env": { "KUBECONFIG": "/absolute/path/to/.kube/master.yaml" }
```

The controller resolves its cluster from `KUBECONFIG` the same way `kubectl`
does (via `controller-runtime`'s config loading), so without this it would
fall back to your default kubeconfig/context instead of the master cluster.
Start the debugger; the controller is now running on your host against the
master cluster, with Cluster Gateway running inside it.

## 5. Create the slave cluster

```bash
k3d cluster create slave --wait
k3d kubeconfig get slave > ~/.kube/slave.yaml
```

## 6. Make the slave cluster reachable from Cluster Gateway

`vela cluster join` hands the slave's kubeconfig to Cluster Gateway, which
then dials the address in that kubeconfig **from inside the master cluster's
pod network**, not from your host. By default k3d writes `server:
https://0.0.0.0:<port>` (or `127.0.0.1`), which resolves to the caller
itself, so from inside a pod that's the pod, not your slave cluster. Edit
`~/.kube/slave.yaml`:

1. Replace the `server:` host with an address reachable from inside the
   master cluster's containers, not `0.0.0.0`/`127.0.0.1`. This is the exact
   same reachability problem `hack/debug-webhook-setup.sh` solves for webhook
   certificate SANs (see [`webhook-debugging.md`](./webhook-debugging.md#what-make-webhook-debug-setup-actually-does)):
   `host.docker.internal` on macOS, the Docker bridge gateway IP on Linux
   bridge networking, or your host's LAN IP as a fallback. Keep the port k3d
   already wrote.
2. Add `insecure-skip-tls-verify: true` under the `cluster` entry.
3. Remove the now-unneeded `certificate-authority-data` line.

```yaml
clusters:
- cluster:
    server: https://<reachable-host-address>:<port>   # was 0.0.0.0 or 127.0.0.1
    insecure-skip-tls-verify: true
  name: k3d-slave
```

If your host itself can't reach `k3d-master`'s or `k3d-slave`'s API server
(for example, running inside a devcontainer), see the networking
troubleshooting note in
[`k3d-workflow.md`](./k3d-workflow.md#troubleshooting) first; the same
container-vs-host address mismatch applies here.

## 7. Join the slave cluster

```bash
export KUBECONFIG=~/.kube/master.yaml   # vela cluster join targets whatever context is current
vela cluster join ~/.kube/slave.yaml --name k3d-slave
vela cluster ls
```

Expected:

```
CLUSTER      ALIAS    TYPE               ENDPOINT                        ACCEPTED
local                 Internal           -                               true
k3d-slave             X509Certificate    https://<reachable-address>:<port>    true
```

## 8. Deploy an app across both clusters

```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: multi-cluster-demo
spec:
  components:
    - name: podinfo
      type: webservice
      properties:
        image: stefanprodan/podinfo:4.0.3
      traits:
        - type: expose
          properties:
            port: [80]
  policies:
    - name: topo
      type: topology
      properties:
        clusters: ["local", "k3d-slave"]
  workflow:
    steps:
      - name: deploy
        type: deploy
        properties:
          policies: ["topo"]
```

```bash
kubectl apply -f multi-cluster-demo.yaml   # against the master cluster (KUBECONFIG=~/.kube/master.yaml)
```

Set a breakpoint before applying (e.g. in the `topology` policy's dispatch
path or wherever you're debugging), then check both clusters:

```bash
KUBECONFIG=~/.kube/master.yaml kubectl get pods
KUBECONFIG=~/.kube/slave.yaml kubectl get pods
```

`podinfo` should be running in both.

## Cleanup

```bash
helm uninstall kubevela -n vela-system --kubeconfig ~/.kube/master.yaml
k3d cluster delete master slave
# the trap from step 3 restores charts/vela-core/templates/kubevela-controller.yaml
```

## Troubleshooting

- **Controller can't reach the cluster from your IDE**: confirm the
  `KUBECONFIG` env var on the run configuration is an absolute path; IDEs
  don't reliably expand `~`.
- **`vela cluster join` succeeds but pods never appear on the slave**: check
  Cluster Gateway's own logs (`kubectl logs -n vela-system -l
  app=kubevela-cluster-gateway --kubeconfig ~/.kube/master.yaml`) for dial
  errors to the address you put in `slave.yaml`, that confirms whether it's a
  reachability problem (step 6) versus a policy/workflow problem.
- **Pods land on `local` but not on the slave cluster name**: the name in
  the `topology` policy's `clusters` list must match the `CLUSTER` column
  from `vela cluster ls`, not an arbitrary alias.

## References

- [Cluster Gateway](https://github.com/kubevela/cluster-gateway)
- [KubeVela multi-cluster docs](https://kubevela.io/docs/case-studies/multi-cluster)

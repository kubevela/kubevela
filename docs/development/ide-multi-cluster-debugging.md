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

## Quick start: automate the cluster setup

Steps 1-2 and 5-7 below (create both clusters, patch their kubeconfigs,
install, join) are mechanical and don't change based on what you're
debugging. A script that does all of it in one shot:

```bash
#!/usr/bin/env bash
# Create a master/slave KubeVela multi-cluster lab on k3d: creates both
# clusters, patches both kubeconfigs for cross-cluster reachability, installs
# KubeVela on master, and joins slave into it.
#
# Usage: bash setup-k3d-multicluster.sh
#
# Optional environment variables:
#   MASTER_NAME=master
#   SLAVE_NAME=slave
#   KUBECONFIG_DIR="$HOME/.kube"
#   HOST_ADDRESS=192.168.1.10   # auto-detected if unset
#   VELA_BIN=./bin/vela
#   VELA_INSTALL_TIMEOUT=300s
#   CLUSTER_READY_TIMEOUT=120s

set -euo pipefail

MASTER_NAME="${MASTER_NAME:-master}"
SLAVE_NAME="${SLAVE_NAME:-slave}"
KUBECONFIG_DIR="${KUBECONFIG_DIR:-$HOME/.kube}"
MASTER_KUBECONFIG="${MASTER_KUBECONFIG:-$KUBECONFIG_DIR/$MASTER_NAME.yaml}"
SLAVE_KUBECONFIG="${SLAVE_KUBECONFIG:-$KUBECONFIG_DIR/$SLAVE_NAME.yaml}"
VELA_INSTALL_TIMEOUT="${VELA_INSTALL_TIMEOUT:-300s}"
CLUSTER_READY_TIMEOUT="${CLUSTER_READY_TIMEOUT:-120s}"

if [[ -x "${VELA_BIN:-}" ]]; then
    VELA="${VELA_BIN}"
elif [[ -x "./bin/vela" ]]; then
    VELA="./bin/vela"
else
    VELA="vela"
fi

info() { printf '\033[1;34m==>\033[0m %s\n' "$*"; }
die() { printf '\033[1;31mERROR:\033[0m %s\n' "$*" >&2; exit 1; }
require_cmd() { command -v "$1" >/dev/null 2>&1 || die "$1 is required but was not found in PATH"; }

detect_host_address() {
    if [[ -n "${HOST_ADDRESS:-}" ]]; then
        printf '%s\n' "$HOST_ADDRESS"
        return
    fi
    if [[ "$(uname -s)" == "Darwin" ]]; then
        ipconfig getifaddr en0 2>/dev/null && return
        ipconfig getifaddr en1 2>/dev/null && return
    fi
    if command -v ip >/dev/null 2>&1; then
        ip route get 1.1.1.1 2>/dev/null | awk '{for (i=1; i<=NF; i++) if ($i == "src") {print $(i+1); exit}}' && return
    fi
    die "could not detect host address; set HOST_ADDRESS explicitly"
}

wait_for_cluster() {
    KUBECONFIG="$1" kubectl wait --for=condition=Ready nodes --all --timeout="$CLUSTER_READY_TIMEOUT"
}

api_port_from_kubeconfig() {
    kubectl --kubeconfig "$1" config view --raw -o jsonpath='{.clusters[0].cluster.server}' \
        | sed -E 's#^https://[^:/]+:([0-9]+).*$#\1#'
}

cluster_entry_from_kubeconfig() {
    kubectl --kubeconfig "$1" config view --raw -o jsonpath='{.clusters[0].name}'
}

patch_kubeconfig_server() {
    local kubeconfig="$1" cluster_name="$2" host_address="$3" port="$4"
    info "Patching $kubeconfig server to https://$host_address:$port"
    kubectl --kubeconfig "$kubeconfig" config set-cluster "$cluster_name" \
        --server="https://$host_address:$port" --insecure-skip-tls-verify=true >/dev/null
    kubectl --kubeconfig "$kubeconfig" config unset "clusters.$cluster_name.certificate-authority-data" >/dev/null 2>&1 || true
}

join_slave_to_master() {
    info "Joining slave cluster into master"
    KUBECONFIG="$MASTER_KUBECONFIG" "$VELA" cluster join "$SLAVE_KUBECONFIG"
    local deadline=$((SECONDS + 180)) joined="k3d-$SLAVE_NAME" status
    while (( SECONDS < deadline )); do
        status="$(KUBECONFIG="$MASTER_KUBECONFIG" "$VELA" cluster list 2>/dev/null || true)"
        printf '%s\n' "$status"
        printf '%s\n' "$status" | awk -v name="$joined" '$1 == name && $0 ~ /true/ {found=1} END {exit found ? 0 : 1}' && return
        sleep 5
    done
    die "slave cluster was not accepted within 180s"
}

require_cmd k3d; require_cmd kubectl; require_cmd "$VELA"
mkdir -p "$KUBECONFIG_DIR"

info "Creating master cluster: $MASTER_NAME"
k3d cluster create "$MASTER_NAME" --wait
k3d kubeconfig get "$MASTER_NAME" > "$MASTER_KUBECONFIG"

info "Creating slave cluster: $SLAVE_NAME"
k3d cluster create "$SLAVE_NAME" --wait
k3d kubeconfig get "$SLAVE_NAME" > "$SLAVE_KUBECONFIG"

host_address="$(detect_host_address)"
patch_kubeconfig_server "$MASTER_KUBECONFIG" "$(cluster_entry_from_kubeconfig "$MASTER_KUBECONFIG")" "$host_address" "$(api_port_from_kubeconfig "$MASTER_KUBECONFIG")"
patch_kubeconfig_server "$SLAVE_KUBECONFIG" "$(cluster_entry_from_kubeconfig "$SLAVE_KUBECONFIG")" "$host_address" "$(api_port_from_kubeconfig "$SLAVE_KUBECONFIG")"

wait_for_cluster "$MASTER_KUBECONFIG"
wait_for_cluster "$SLAVE_KUBECONFIG"

info "Installing KubeVela on master"
KUBECONFIG="$MASTER_KUBECONFIG" "$VELA" install
KUBECONFIG="$MASTER_KUBECONFIG" kubectl wait deployment -n vela-system --all --for=condition=Available --timeout="$VELA_INSTALL_TIMEOUT"

join_slave_to_master

cat <<EOF

Done.
Master kubeconfig: $MASTER_KUBECONFIG
Slave kubeconfig:  $SLAVE_KUBECONFIG

Use:
  export KUBECONFIG=$MASTER_KUBECONFIG
  $VELA cluster list
EOF
```

A few things to know before running it:

- **It patches both kubeconfigs, not just the slave's**, unlike the more
  conservative "only the slave needs it" reasoning in step 6 below. That's
  the safer default: on some Docker/network setups the host itself can't
  reach k3d's `0.0.0.0`/`127.0.0.1` server address either, so patching only
  the slave isn't always enough.
- **`vela install` pulls the last released chart from KubeVela's chart repo,
  not your local `./charts/vela-core`.** This script gets you a working
  master/slave topology to test joins, `topology` policies, or Cluster
  Gateway behavior against a stock release. It does **not** run your local
  code. If you also want to debug the controller from your IDE, follow
  steps 3-4 below (strip the `Deployment`, install with the local chart)
  instead of, or in addition to, this script's `vela install` step.
- **It's your own script to adapt.** Unlike `hack/debug-webhook-setup.sh`,
  this isn't wired into the Makefile, drop it wherever's convenient and run
  it with `bash`.

Cleanup:

```bash
k3d cluster delete slave master
rm -f ~/.kube/master.yaml ~/.kube/slave.yaml
```

> The original version of this script also deletes *every* existing k3d
> cluster on the host before creating master/slave, as a "start clean"
> convenience. That's a sharp edge if you have unrelated k3d clusters
> around, it's been left out of the version above; add it back deliberately
> (`k3d cluster delete --all` before the create steps) only if you actually
> want a fully clean slate.

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
at all, the cluster's own kubeconfig may need the same server-address fix
before this step will work.

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

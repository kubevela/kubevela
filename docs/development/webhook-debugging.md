# Debugging the Admission Webhook

KubeVela's admission webhook validates `ComponentDefinition`, `TraitDefinition`,
`PolicyDefinition`, `WorkflowStepDefinition`, and `Application` resources at
create/update time. For example, it checks that a CUE template only
references Kubernetes resources that actually exist on the cluster. It also
mutates (defaults/normalizes) `Application` and `ComponentDefinition`
resources before validation runs. This guide covers debugging both the
validating and mutating handlers. For general controller debugging, see
[`ide-debugging.md`](./ide-debugging.md).

The webhook needs a TLS-serving certificate the API server trusts, and
webhook configurations pointing at wherever the webhook server is running.
Locally, that means running the webhook server on your host (via your IDE)
and pointing the cluster's webhook config back at your machine.

Run `make webhook-help` at any time for a quick cheat-sheet of the commands
below, printed straight from the Makefile.

## Quick start

```bash
make webhook-debug-setup
```

Then start the **"Debug Webhook Validation"** configuration from your IDE
(see [`ide-debugging.md`](./ide-debugging.md)).

### What `make webhook-debug-setup` actually does

1. `make k3d-create`: creates a k3d cluster (default name `kubevela-debug`,
   1 server + 1 agent). This target is a generic "give me a k3d cluster"
   helper, not webhook-specific. It's the same one covered in
   [`k3d-workflow.md`](./k3d-workflow.md#1-create-the-cluster), so you can
   also use it (or `make k3d-delete`) outside of webhook debugging, or skip
   it entirely and point this whole workflow at a cluster you already have.
2. `make manifests && kubectl apply -f charts/vela-core/crds/`: installs
   KubeVela's CRDs.
3. `make webhook-setup`, which runs `hack/debug-webhook-setup.sh`:
   - Generates a CA and a server certificate/key under
     `k8s-webhook-server/serving-certs/`.
   - **Auto-detects a reachable host address** for the certificate's SANs and
     the webhook config's URL: it checks whether the current kubectl context
     is a k3d cluster, inspects whether that cluster's Docker network mode is
     `host` or `bridge`, and picks accordingly (`host.docker.internal` on
     macOS, `127.0.0.1` for Linux host-networking, the Docker bridge gateway
     IP for Linux bridge-networking, or a Rancher-Desktop-style default
     gateway if it isn't a k3d cluster at all). The certificate's SAN list
     also always includes `localhost`, `host.k3d.internal`,
     `host.docker.internal`, and `host.lima.internal` so it works across the
     common local container-runtime setups.
   - Creates the `webhook-server-cert` TLS secret in the `vela-system`
     namespace.
   - Creates a `ValidatingWebhookConfiguration` named
     `kubevela-vela-core-admission` covering componentdefinitions,
     traitdefinitions, policydefinitions, workflowstepdefinitions, and
     applications, each pointing at `https://<detected-host>:<port>/...`
     with `failurePolicy: Fail`.
   - Creates a `MutatingWebhookConfiguration`, also named
     `kubevela-vela-core-admission`, covering applications and
     componentdefinitions (the only two resource kinds the controller
     mutates), same host/port/CA bundle as the validating config.

## Manual / step-by-step equivalent

Useful if you want to point the webhook at a specific address the
auto-detection doesn't pick, or if you already have a cluster and just need
the certs/config:

```bash
make k3d-create                                     # or use an existing cluster
make manifests && kubectl apply -f charts/vela-core/crds/ --validate=false
make webhook-setup                                  # runs hack/debug-webhook-setup.sh with no arguments (port 9445)
```

`make webhook-setup`'s recipe calls the script with no arguments, so it
can't forward a custom port. For a non-default port, call the script
directly instead of through `make`:

```bash
./hack/debug-webhook-setup.sh 9446
```

Default is `9445`, not `9443`: if you're using Rancher Desktop, it already
binds `9443` on your host, so a webhook server configured to listen on that
port will fail to start.

> If your cluster isn't k3d, the script's host-address auto-detection falls
> back to a hardcoded `192.168.5.2`, which is almost certainly not reachable
> from that cluster. There's no flag or environment variable to override
> it, the script only takes the port as an argument, so on a non-k3d
> cluster you'll need to edit the `HOST_IP="192.168.5.2"` fallback in
> `hack/debug-webhook-setup.sh` to your actual reachable address before
> running it.

## Recommended breakpoints

**Validating handlers:**
- `pkg/webhook/core.oam.dev/v1beta1/application/validating_handler.go`
- `pkg/webhook/core.oam.dev/v1beta1/componentdefinition/component_definition_validating_handler.go`
- `pkg/webhook/core.oam.dev/v1beta1/traitdefinition/validating_handler.go`
- `pkg/webhook/core.oam.dev/v1beta1/policydefinition/validating_handler.go`
- `pkg/webhook/core.oam.dev/v1beta1/workflowstepdefinition/workflowstep_validating_handler.go`

**Mutating handlers:**
- `pkg/webhook/core.oam.dev/v1beta1/application/mutating_handler.go`
- `pkg/webhook/core.oam.dev/v1beta1/componentdefinition/mutating_handler.go`

## Verifying the webhook server is listening

Before applying a real resource, confirm the webhook server itself is up and
reachable at the address the cluster will call. This isolates "my webhook
server isn't running/reachable" from "the cluster isn't calling it" as two
separate failure modes:

```bash
curl -s --cacert k8s-webhook-server/serving-certs/ca.crt \
  -X POST "https://<detected-host>:<port>/mutating-core-oam-dev-v1beta1-componentdefinitions?timeout=10s"
```

Use `--cacert` with the CA `hack/debug-webhook-setup.sh` generated, not
`-k`. `-k` skips certificate validation entirely, so it can "succeed" even
with a cert the API server would actually reject (wrong SAN, wrong CA),
which defeats the point of this check.

A correctly running server responds with something like:

```json
{"response":{"uid":"","allowed":false,"status":{"metadata":{},"message":"request body is empty","code":400}}}
```

Getting a response at all (even this "rejected" one) confirms the server is
listening and its TLS certificate is valid. A connection error or timeout
here means the problem is in the server/network setup, not in your
breakpoint or the resource you're about to apply.

## Triggering a breakpoint

With the debugger running and a breakpoint set, apply any resource of the
matching kind:

```bash
kubectl apply -f your-componentdefinition.yaml
```

Execution should stop at your breakpoint once the API server calls out to
your local webhook server.

## Cleaning up

```bash
make webhook-clean    # removes local certs, the Secret, and both webhook configurations
make k3d-delete        # deletes the k3d cluster entirely
```

## Troubleshooting

- **Connection refused / webhook never triggers**: confirm the debugger is
  actually running and listening on the configured port, and that the
  webhook configuration's URL matches an address reachable from inside the
  cluster (`kubectl get validatingwebhookconfigurations
  kubevela-vela-core-admission -o yaml`, or `mutatingwebhookconfigurations`
  for the mutating side). The auto-detected address can be wrong in less
  common Docker/network setups. Rerun `hack/debug-webhook-setup.sh` after
  adjusting your setup, or edit the generated webhook config directly. Use
  the curl check above to confirm the server side before suspecting the
  cluster-side config.
- **TLS errors**: regenerate certificates (`make webhook-setup`) and restart
  the debugger. Stale certs are a common cause after switching networks or
  Docker runtimes.
- **Everything hangs after you stop debugging**: the webhook config uses
  `failurePolicy: Fail`, which means an unreachable webhook blocks *all*
  create/update operations on the covered resource kinds, cluster-wide.
  Run `make webhook-clean` as soon as you're done, or before switching to a
  different debugging task.

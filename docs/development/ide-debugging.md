# Debugging from Your IDE

Run `cmd/core/main.go` directly on your host, attached to your IDE's
debugger, against any cluster your kubeconfig can reach (a local k3d cluster
or a remote one). This is the fastest inner loop because there's no image to
build, but it doesn't test the actual container image, so pair it with
[the k3d workflow](./k3d-workflow.md) before merging.

## Prerequisites

You need a cluster with KubeVela's CRDs and definitions installed:

```bash
# Against any cluster your current kubeconfig points to:
make core-install   # applies CRDs from charts/vela-core/crds/
make def-install     # installs default ComponentDefinitions/TraitDefinitions

# Or spin up a local cluster first (see k3d-workflow.md)
```

## VS Code

1. Install the [Go extension](https://marketplace.visualstudio.com/items?itemName=golang.Go).
2. Add configurations to `.vscode/launch.json` (create the file if it doesn't exist):

   ```json
   {
       "version": "0.2.0",
       "configurations": [
           {
               "name": "Run KubeVela Core",
               "type": "go",
               "request": "launch",
               "mode": "debug",
               "program": "${workspaceFolder}/cmd/core",
               "args": [
                   "-v=3",
                   "--dev-logs=true",
                   "--log-file-path=${workspaceFolder}/vela.log",
                   "--application-re-sync-period=1m"
               ]
           },
           {
               "name": "Debug Webhook Validation",
               "type": "go",
               "request": "launch",
               "mode": "debug",
               "program": "${workspaceFolder}/cmd/core",
               "args": [
                   "--dev-logs=true",
                   "--log-debug=true",
                   "--metrics-addr=:8080",
                   "--enable-leader-election=false",
                   "--use-webhook=true",
                   "--webhook-port=9445",
                   "--webhook-cert-dir=${workspaceFolder}/k8s-webhook-server/serving-certs",
                   "--application-re-sync-period=1m"
               ],
               "env": {
                   "KUBECONFIG": "${env:HOME}/.kube/config",
                   "POD_NAMESPACE": "vela-system"
               },
               "console": "integratedTerminal"
           }
       ]
   }
   ```

3. Set breakpoints (see below), open the "Run and Debug" panel, pick a
   configuration, and press `F5`.

The first configuration runs the controller with no webhook server. That's the
common case for day-to-day reconciler work. The second additionally starts
the webhook server on `:9445`; see [`webhook-debugging.md`](./webhook-debugging.md)
for the certificate and cluster-side setup it needs.

There's no `--webhook-timeout` flag on the controller side, don't add one,
`pflag` rejects unregistered flags and the process won't start at all. The
webhook call's timeout is a cluster-side setting: `timeoutSeconds` on each
rule in the `ValidatingWebhookConfiguration`/`MutatingWebhookConfiguration`
objects that `hack/debug-webhook-setup.sh` creates.

## IntelliJ IDEA / GoLand

1. Install the Go plugin (bundled in GoLand; install from Marketplace in
   IntelliJ IDEA Ultimate).
2. **Run → Edit Configurations → + → Go Build**.
3. Fill in:
   - **Run kind**: `Package`
   - **Package path**: `github.com/oam-dev/kubevela/cmd/core`
   - **Working directory**: the repository root
   - **Program arguments**: same flags as the VS Code config above, e.g.
     `-v=3 --dev-logs=true --log-file-path=./vela.log --application-re-sync-period=1m`
   - **Environment variables**: for the webhook variant, add
     `KUBECONFIG=<path to your kubeconfig>` and `POD_NAMESPACE=vela-system`
4. Save it under a descriptive name (e.g. "Run KubeVela Core" /
   "Debug Webhook Validation" to mirror the VS Code configs), then click the
   **Debug** icon (not Run) next to the configuration picker.

## Recommended breakpoints

- **General controller behavior**: `Reconcile()` in
  `pkg/controller/core.oam.dev/v1beta1/application/application_controller.go`.
- **Webhook validation**: the handlers under
  `pkg/webhook/core.oam.dev/v1beta1/*/validating_handler.go`, e.g.
  `application/validating_handler.go` or
  `componentdefinition/component_definition_validating_handler.go`.

## Verifying it's working

- **No-webhook config**: apply an `Application` (`kubectl apply -f ...`) and
  confirm execution stops at your `Reconcile()` breakpoint.
- **Webhook config**: once the webhook server has started (check the IDE's
  console for a listening message), applying a `ComponentDefinition` or
  `Application` should stop at your validating-handler breakpoint. If nothing
  happens, the webhook configuration likely isn't pointing at your machine.
  See [`webhook-debugging.md`](./webhook-debugging.md).

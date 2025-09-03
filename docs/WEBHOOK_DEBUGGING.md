# KubeVela Webhook Debugging Guide

This guide explains how to debug KubeVela webhook validation locally, particularly for the feature that validates ComponentDefinitions, TraitDefinitions, and PolicyDefinitions to ensure they don't reference non-existent CRDs.

## Overview

The webhook validation feature checks that CUE templates in definitions only reference Kubernetes resources that exist on the cluster. This prevents runtime errors when non-existent CRDs are referenced.

## Prerequisites

- Docker Desktop or similar container runtime
- k3d for local Kubernetes clusters
- VS Code with Go extension
- kubectl configured
- openssl for certificate generation

## Quick Start

```bash
# 1. Complete setup (cluster + CRDs + webhook)
make webhook-debug-setup

# 2. Start VS Code debugger
# Press F5 and select "Debug Webhook Validation"
```

## Detailed Setup Steps

### 1. Environment Setup

```bash
# Create k3d cluster
make k3d-create

# Install KubeVela CRDs
make manifests
kubectl apply -f charts/vela-core/crds/
```

### 2. Webhook Certificate Setup

The webhook requires TLS certificates with proper Subject Alternative Names (SANs) for IP addresses.

```bash
# Generate certificates and create Kubernetes secret
make webhook-setup
```

This creates:
- CA certificate and key
- Server certificate with IP SANs (127.0.0.1, Docker internal IP, local machine IP)
- Kubernetes Secret `webhook-server-cert` in `vela-system` namespace
- ValidatingWebhookConfiguration pointing to local debugger

### 3. Start Debugger in VS Code

#### VS Code Launch Configuration

If you're using VSCode add this configuration to `.vscode/launch.json`:

```json
{
    "version": "0.2.0",
    "configurations": [
        {
            "name": "Debug Webhook Validation",
            "type": "go",
            "request": "launch",
            "mode": "debug",
            "program": "${workspaceFolder}/cmd/core",
            "args": [
                "--log-debug=true",
                "--metrics-addr=:8080",
                "--enable-leader-election=false",
                "--use-webhook=true",
                "--webhook-port=9445",
                "--webhook-cert-dir=${workspaceFolder}/k8s-webhook-server/serving-certs"
            ],
            "env": {
                "KUBECONFIG": "${env:HOME}/.kube/config",
                "POD_NAMESPACE": "vela-system"
            },
            "showLog": false,
            "console": "integratedTerminal"
        }
    ]
}
```

#### Set Breakpoints

Recommended breakpoint locations:
- `pkg/webhook/core.oam.dev/v1beta1/componentdefinition/validating_handler.go` - ComponentDefinition handler
- `pkg/webhook/core.oam.dev/v1beta1/traitdefinition/validating_handler.go` - TraitDefinition handler
- `pkg/webhook/core.oam.dev/v1beta1/policydefinition/validating_handler.go` - PolicyDefinition handler
- `pkg/webhook/core.oam.dev/v1beta1/workflowstepdefinition/workflowstep_validating_handler.go` - WorkflowDefinition handler

#### Launch Debugger

1. Open VS Code
2. Press `F5` or go to Run → Start Debugging
3. Select **"Debug Webhook Validation"** configuration
4. Wait for webhook server to start (look for message about port 9445)

The debugger configuration includes:
- `--use-webhook=true` - Enables webhook server
- `--webhook-port=9445` - Port for webhook server
- `--webhook-cert-dir` - Path to certificates
- `POD_NAMESPACE=vela-system` - Required for finding the secret

## Make Targets Reference

| Target | Description |
|--------|-------------|
| `make webhook-help` | Show webhook debugging help |
| `make webhook-debug-setup` | Complete setup (cluster + CRDs + webhook) |
| `make k3d-create` | Create k3d cluster |
| `make k3d-delete` | Delete k3d cluster |
| `make webhook-setup` | Setup certificates and webhook configuration |
| `make webhook-clean` | Clean up webhook environment |

## Troubleshooting

### Connection Refused Error

If you get "connection refused" errors:
1. Ensure the debugger is running in VS Code
2. Check that port 9445 is not blocked by firewall
3. Verify the webhook server started (check VS Code console)

### TLS Certificate Errors

If you get certificate validation errors:
1. Regenerate certificates: `make webhook-setup`
2. Restart the debugger
3. Ensure IP addresses in certificates match your setup

### Webhook Not Triggering

If the webhook doesn't trigger:
1. Check ValidatingWebhookConfiguration: `kubectl get validatingwebhookconfiguration`
2. Verify the webhook URL matches your debugger's IP
3. Check namespace is correct (vela-system)

### Secret Not Found

If you see "Wait webhook secret" messages:
1. Ensure the secret exists: `kubectl get secret webhook-server-cert -n vela-system`
2. Recreate if needed: `make webhook-setup`

## How It Works

1. **Certificate Generation**: Creates TLS certificates with proper SANs for local IPs
2. **Secret Creation**: Stores certificates in Kubernetes secret
3. **Webhook Configuration**: Creates ValidatingWebhookConfiguration pointing to local debugger
4. **Debugger Startup**: VS Code starts the webhook server on port 9445
5. **Validation**: When definitions are applied, Kubernetes calls the webhook
6. **Debugging**: Breakpoints allow stepping through validation logic

## Files and Components

- **Script**: `hack/debug-webhook-setup.sh` - Main setup script
- **Makefile**: `makefiles/develop.mk` - Make targets for debugging
- **VS Code Config**: `.vscode/launch.json` - Debugger configuration
- **Test Files**: `test/webhook-*.yaml` - Test manifests
- **Validation Logic**: `pkg/webhook/utils/utils.go` - Core validation implementation
- **Handlers**: `pkg/webhook/core.oam.dev/v1beta1/*/validating_handler.go` - Resource handlers

## Clean Up

```bash
# Clean up webhook setup
make webhook-clean

# Delete k3d cluster
make k3d-delete
```

## Tips

1. **Always start the debugger before testing** - The webhook configuration points to your local machine
2. **Use breakpoints wisely** - Too many breakpoints can cause timeouts
3. **Check logs** - VS Code Debug Console shows detailed logs
4. **Test both valid and invalid cases** - Ensures validation works correctly
5. **Keep certificates updated** - Regenerate if IPs change

## Related Documentation

- [KubeVela Webhook Implementation](../pkg/webhook/README.md)
- [CUE Template Validation](../pkg/webhook/utils/README.md)
- [Admission Webhooks](https://kubernetes.io/docs/reference/access-authn-authz/extensible-admission-controllers/)
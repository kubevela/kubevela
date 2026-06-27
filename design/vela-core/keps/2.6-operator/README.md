# KEP-2.6: KubeVela Operator

**Status:** Drafting (Not ready for consumption)
**Parent:** [vNext Roadmap](../README.md)

A `KubeVela` CR that describes and reconciles the entire KubeVela installation for a given cluster. The operator installs, configures, and upgrades the appropriate controllers based on the declared role.

This follows the industry direction of operator-managed installations (Cert-Manager, Istio/Sail, OpenTelemetry) — a single CR in git describes the complete installation, the operator makes it so and keeps it converged.

## KubeVela CR

```yaml
apiVersion: core.oam.dev/v1alpha1
kind: KubeVela
metadata:
  name: kubevela
  namespace: vela-system
spec:
  role: hub                    # hub | spoke | standalone
  version: "2.0.0"

  # spoke: only valid when role: spoke
  # Validated by admission webhook — rejected if role != spoke
  spoke:
    name: "prod-cluster-1"
    hubEndpoint: "https://hub.example.com"

  components:
    applicationController:     # hub only
      enabled: true
      replicas: 2
    componentController:       # spoke + standalone
      enabled: true
      replicas: 3
    workflowRunController:     # all roles, default on
      enabled: true

  features:
    multiCluster: true
    ocmIntegration: true
    driftDetection: true
    admissionWebhook: true

  security:
    keyRotationInterval: "24h"
    tokenTTL: "1h"
    auditEvents: true

  addons:                      # references only — Addon CR handles lifecycle
    - name: velaux
      enabled: true
    - name: ocm-hub-control-plane
      enabled: true
```

## Validation

A validating admission webhook enforces role-specific field constraints:

| Role | Valid fields | Invalid fields |
|---|---|---|
| `hub` | `components`, `features`, `security`, `addons` | `spoke` |
| `spoke` | `components`, `features`, `security`, `addons`, `spoke` | — |
| `standalone` | `components`, `features`, `security`, `addons` | `spoke` |

## Operator Responsibilities

- **Install** — deploys the correct controllers for the declared role
- **Configure** — applies feature flags and security settings to controller deployments
- **Upgrade** — orchestrates safe rolling upgrades when `version` changes; understands controller dependency ordering
- **Drift correction** — reconciles manual edits back to desired state
- **Spoke registration** — when `role: spoke`, drives the bootstrap handshake automatically (no manual CLI steps required)
- **Status** — surfaces installation health in `KubeVela.status`

```yaml
status:
  ready: true
  version: "2.0.0"
  role: hub
  components:
    applicationController:
      ready: true
      version: "2.0.0"
    componentController:
      ready: true
      version: "2.0.0"
    workflowRunController:
      ready: true
      version: "2.0.0"
  addons:
    - name: velaux
      ready: true
  conditions: [...]
```

## Benefits over Helm-only Installation

| Concern | Helm | KubeVela Operator |
|---|---|---|
| Upgrades | Template-and-apply, no state awareness | Orchestrated rolling upgrade with dependency ordering |
| Configuration drift | Not reconciled | Continuously reconciled back to desired state |
| Spoke registration | Manual CLI steps | Automatic on `role: spoke` |
| Observability | Hunt through Helm releases | `kubectl get kubevela` shows complete state |
| GitOps | Hundreds of Helm values | One CR, readable and reviewable |

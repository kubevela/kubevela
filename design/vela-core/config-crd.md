# CRD-based Config Management (`config.oam.dev/v1alpha1`)

- Owner: Anish Bista (@anishbista60)
- Date: 2026-08-12
- Status: Implementing

### Introduction

This proposal introduces a new Kubernetes API group, `config.oam.dev/v1alpha1`, with two CRDs — `ConfigTemplate` and `Config` — backed by controllers and a validating webhook. It replaces the current pattern of storing config templates as labeled `ConfigMap`s and configs as labeled `Secret`s, while keeping the existing ConfigMap/Secret convention fully working as a fallback.

### Background

KubeVela's Config Management (`vela config` / `vela config-template`) today has no Kubernetes API types of its own. A `ConfigTemplate` is a `ConfigMap` named `config-template-<name>` carrying a `config.oam.dev/catalog: velacore-config` label, and a `Config` is a `Secret` with the same label. All of the logic that creates, validates and renders them lives in `pkg/config.Factory`, which is called directly by the `vela` CLI.

This has three problems:

1. **No proper API types.** `kubectl get configs` returns nothing meaningful — configs are indistinguishable from any other Secret without inspecting labels. There's no schema, no `kubectl explain`, no discoverability, and no clean integration point for GitOps tooling or kubectl plugins.
2. **Validation only runs in the CLI.** CUE schema validation (`script.ValidatePropertiesWithCueX`) and the custom `template.validation.$returns` check (in `pkg/config/factory.go`) run client-side, inside `Factory.ParseConfig`. A `Config`/Secret created through any other path — a workflow step, VelaUX, or a direct `kubectl apply` — bypasses validation entirely, so the same template can silently enforce different rules depending on how the config was created.
3. **No native separation of secret input from resolved output.** Property values supplied by the user and the materialized output are both stored as a single Secret, and there's no first-class way to source sensitive property values from an existing Secret instead of putting them in the config object itself.

### Goals & Non-Goals

**Goals**

- Introduce `ConfigTemplate` and `Config` as proper CRDs with controllers, so `kubectl get`/`describe`/`explain` work and the objects have real status/conditions.
- Enforce the same validation (CUE schema, `template.validation.$returns`, mutual exclusivity of `properties`/`propertiesFrom`) for every entry point — CLI, workflow step, VelaUX, or raw `kubectl apply` — via an admission webhook, instead of only in the CLI.
- Allow a `Config`'s properties to be sourced from an existing `Secret` (`spec.propertiesFrom.secretRef`) instead of being embedded inline, for sensitive values.
- Preserve full backward compatibility: existing `config-template-*` ConfigMaps and Secret-based Configs, and addons that install them, continue to work unchanged.

**Non-Goals**

- A migration tool that converts legacy ConfigMap-based templates into `ConfigTemplate` CRDs. Both forms are expected to coexist.
- A deprecation timeline for the legacy ConfigMap/Secret convention.
- Changes to multi-cluster config distribution. Phase 1 keeps the existing Application-based distribution mechanism (`Factory.CreateOrUpdateDistribution`) unchanged and orthogonal to this proposal.

### Proposal

#### API types

Two namespaced CRDs are added under `apis/config.oam.dev/v1alpha1/`:

```go
type ConfigTemplateSpec struct {
    Template    string               // CUE template, same format as the legacy config-template-* ConfigMap's "template" key
    Scope       ConfigTemplateScope  // "system" | "namespace", defaults to "namespace"
    Sensitive   bool                 // Configs from this template can't be read back through CLI/workflow
    Alias       string
    Description string
}

type ConfigSpec struct {
    TemplateRef    *ConfigTemplateReference // name (+ optional namespace, defaults to vela-system)
    Properties     *runtime.RawExtension    // inline properties; mutually exclusive with PropertiesFrom
    PropertiesFrom *PropertiesReference     // secretRef{name, key}; key defaults to "properties"
    Alias          string
    Description    string
}
```

`Config.status.phase` is `Available`/`Error` with a `secretRef` pointing at the materialized output Secret. `ConfigTemplate.status` carries the same phase plus an OpenAPI v3 `schema` extracted from `template.parameter`, used to validate `Config` properties against.

Sensitive values are never stored in the `Config` object: `spec.propertiesFrom.secretRef` points at a Secret the controller reads at reconcile time, so the CRD itself only ever holds a reference, not the value.

#### Controllers

Both reconcilers (`pkg/controller/config.oam.dev/v1alpha1/{config,configtemplate}`) are purely event-driven — no `RequeueAfter` — matching the existing `ComponentDefinition`/`TraitDefinition` pattern.

- **`ConfigTemplateReconciler`**: parses the CUE template, extracts an OpenAPI schema from `template.parameter`, and writes it to `status.schema`. Marks `Error` phase (with a condition) on parse failure, `Available` on success.
- **`ConfigReconciler`**:
  1. Resolves `spec.templateRef` — CRD first, falling back to the legacy `config-template-<name>` ConfigMap if no CRD exists (see below). If the CRD exists but hasn't reconciled to `Available` yet, reconciliation is deferred by marking the Config `Error` and relying on the ConfigTemplate watch to retrigger it.
  2. Resolves properties, either from `spec.properties` inline or by reading `spec.propertiesFrom.secretRef`.
  3. Evaluates the CUE template against the properties (`template.output`, `template.validation.$returns`), same evaluation path the legacy `Factory.ParseConfig` uses.
  4. Applies the materialized output as a `Secret` it owns via `controllerutil.SetControllerReference`, so deleting the `Config` garbage-collects the Secret. It refuses to adopt a pre-existing Secret it doesn't already control, to avoid a Config author overwriting an unrelated Secret via a name collision.
  5. Sets `status.phase`, `status.secretRef`, and a `ReconcileSuccess`/`ReconcileError` condition.

  The controller also watches `ConfigTemplate` objects, `Secret`s referenced via `propertiesFrom`, and legacy `config-template-*` ConfigMaps, re-enqueuing any `Config` that depends on the changed object.

#### Validating webhook

A validating webhook on `Config` (`pkg/webhook/config.oam.dev/v1alpha1/config`) mirrors the controller's template/property resolution and, on create/update, enforces:

| Validation | Before | After |
|---|---|---|
| `properties` match the template's CUE schema | CLI only (`script.ValidatePropertiesWithCueX`) | Webhook |
| Custom `template.validation.$returns` check | CLI only (`factory.go`) | Webhook |
| `properties` / `propertiesFrom` mutual exclusivity | Not enforced | Webhook |
| `ConfigTemplate` CUE syntax is parseable | CLI only | Webhook (`ConfigTemplate` webhook, on the template itself) |

This means every entry point — CLI, workflow step, VelaUX, or a raw `kubectl apply` — is admitted or rejected under the same rules with the same error messages, closing the gap described in Background.

Validation is skipped (admitted) when the referenced template can't yet be resolved (e.g. `ConfigTemplate` not `Available`, or neither CRD nor legacy ConfigMap exists) — the reconciler is the source of truth for that failure mode and will mark the `Config` as `Error`, rather than the webhook rejecting the write outright.

#### Backward compatibility: dual-read, single-write

- `ConfigReconciler` and the validating webhook both read **either** a `ConfigTemplate` CRD **or** a legacy `config-template-<name>` ConfigMap, CRD taking priority when both exist.
- `pkg/config.Factory.LoadTemplate` (used by the CLI and workflow `config` provider) follows the same CRD-first, ConfigMap-fallback resolution, so old and new templates are interchangeable from the caller's point of view.
- Old CLI versions, and addons that install ConfigMap-based templates, are unaffected — they keep writing ConfigMaps/Secrets directly and keep working.
- New CLI versions (`vela config-template apply/list/show/delete`, `vela config create/list/delete`) write the new CRDs; `references/cli/config_crd.go` falls back to the legacy factory-based path if the CRD API isn't installed on the cluster (e.g. an older KubeVela control plane).
- A migration tool to convert legacy ConfigMaps into `ConfigTemplate` CRDs is explicitly out of scope (see Non-Goals) — both representations are expected to coexist indefinitely.

### Examples

```yaml
apiVersion: config.oam.dev/v1alpha1
kind: ConfigTemplate
metadata:
  name: image-registry
  namespace: vela-system
spec:
  scope: namespace
  template: |
    template: {
      parameter: {
        registry: string
        username: string
        password: string
      }
      output: {
        apiVersion: "v1"
        kind:       "Secret"
        data: {
          registry: parameter.registry
          username: parameter.username
          password: parameter.password
        }
      }
    }
---
apiVersion: config.oam.dev/v1alpha1
kind: Config
metadata:
  name: my-registry
  namespace: default
spec:
  templateRef:
    name: image-registry
    namespace: vela-system
  propertiesFrom:
    secretRef:
      name: my-registry-credentials
      key: properties
```

`kubectl get configtemplates -n vela-system` and `kubectl describe config my-registry -n default` now show real objects with `SCOPE`/`PHASE`/`TEMPLATE` columns, instead of requiring label-filtered `kubectl get configmaps/secrets`.

### Progress / Timeline / Milestones

- [x] `ConfigTemplate`/`Config` API types, CRD manifests, deepcopy.
- [x] `ConfigTemplateReconciler`, `ConfigReconciler`, legacy ConfigMap/Secret fallback.
- [x] Validating webhooks for `Config` and `ConfigTemplate`.
- [x] `vela config` / `vela config-template` CLI switched to the CRDs, with fallback to the legacy factory path.
- [ ] Migration tool for ConfigMap → `ConfigTemplate` (future follow-up, out of scope here).
- [ ] Deprecation timeline for the legacy ConfigMap/Secret convention (future follow-up, out of scope here).

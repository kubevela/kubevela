# Addon as a component

Install an addon by declaring it in an Application instead of running
`vela addon enable`. The `addon` ComponentDefinition renders the addon's own
Application as an inner Application that the outer one owns, so the addon's
whole footprint, including its definitions, is tracked and drift corrected like
any other resource.

The examples here use addons from the community registry the `vela-core` chart
pre-registers, so they work on a fresh cluster with no registry setup.

| File | What it shows |
| --- | --- |
| `minimal.yaml` | The shortest form. Addon name comes from the component name. |
| `pinned.yaml` | Addon, version and registry given explicitly. |
| `with-parameters.yaml` | Passing the addon's own parameters through. |

## Enabling the feature

`EnableAddonComponent` is alpha and off by default. It ships as a chart value:

```shell
helm upgrade --install vela-core kubevela/vela-core \
  -n vela-system --create-namespace \
  --set featureGates.enableAddonComponent=true
```

Applying a `type: addon` component with the gate off is refused at admission:

```
admission webhook "validating.core.oam.dev.v1beta1.applications" denied the request:
  1) "schematic": addon-as-component is disabled; enable the
     EnableAddonComponent feature gate to use type: addon components.
```

## Parameters

| Field | Default | Meaning |
| --- | --- | --- |
| `addon` | the component name | Addon to install. |
| `version` | empty | Exact version. Empty resolves to latest, which is not reproducible. |
| `registry` | empty | Registry name. Empty searches the configured registries. |
| `properties` | `{}` | The addon's own parameters, passed to its templates. |
| `skipVersionValidation` | `false` | Skip the addon's vela and kubernetes requirement check. |

## Trying it

```shell
kubectl apply -f pinned.yaml
```

You get two Applications. The one you applied, and an inner `addon-<name>` it
owns:

```shell
$ kubectl -n vela-system get applications
NAME               COMPONENT   TYPE          PHASE     HEALTHY   STATUS
addon-fluxcd       fluxcd-ns   k8s-objects   running   true      Ready:1/1
fluxcd-addon       fluxcd      addon         running   true

$ kubectl -n vela-system get app fluxcd-addon \
    -o jsonpath='{range .status.appliedResources[*]}{.kind}/{.namespace}/{.name}{"\n"}{end}'
Application/vela-system/addon-fluxcd
```

The inner Application holds the addon's resources plus a `k8s-objects`
component for each category of auxiliary the addon ships:

```shell
$ kubectl -n vela-system get app addon-fluxcd \
    -o jsonpath='{range .spec.components[*]}{.name}{"\t"}{.type}{"\n"}{end}'
fluxcd-ns                            k8s-objects
fluxcd-rbac                          k8s-objects
fluxcd-CRD                           k8s-objects
fluxcd-helm-controller               webservice
fluxcd-source-controller             webservice
fluxcd-image-automation-controller   webservice
fluxcd-image-reflector-controller    webservice
fluxcd-kustomize-controller          webservice
addon-definitions                    k8s-objects
addon-config-templates               k8s-objects
addon-schemas                        k8s-objects
```

`addon-definitions`, `addon-config-templates`, `addon-schemas`, `addon-views`,
`addon-secret` and `addon-auxiliaries` appear only when the addon has that
content.

Because the definitions are components, they carry the tracking label and are
restored if something removes them:

```shell
$ kubectl get componentdefinition,traitdefinition -A -l app.oam.dev/name=addon-fluxcd \
    -o custom-columns='NS:.metadata.namespace,KIND:.kind,NAME:.metadata.name' --no-headers
flux-system   ComponentDefinition   helm
flux-system   ComponentDefinition   kustomize
flux-system   TraitDefinition       helm-labels
flux-system   TraitDefinition       kustomize-json-patch
flux-system   TraitDefinition       kustomize-patch
flux-system   TraitDefinition       kustomize-strategy-merge

$ kubectl -n flux-system delete traitdefinition kustomize-patch
$ # reappears within one --application-re-sync-period
```

Removing the outer Application removes the addon:

```shell
kubectl -n vela-system delete application fluxcd-addon
```

## Notes

An addon can be owned by only one installer. `vela addon enable` and a
`type: addon` component both render `addon-<name>`, so the second one to be
applied fails its pre-dispatch dryrun with `existing object Application
vela-system/addon-<name> is managed by other application ...`. Disable the
imperative install before converting an addon to a component.

An addon that declares no `apply-once` policy of its own gets an implicit
`addon-component-state-keep` policy that disables apply-once, so its resources
stay drift corrected. An addon that declares one keeps its own.

`skipVersionValidation: true` bypasses the addon's declared vela and kubernetes
requirements. The check already fails open when it cannot be evaluated, for
example on a controller built without a semver version, so reach for this only
when a requirement is genuinely wrong for your cluster.

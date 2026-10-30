# Module placement

Install a module from an Application and choose which clusters receive the
objects marked runtime. Definitions, and anything marked hub, stay on the hub
cluster. The hub is the cluster where KubeVela itself runs.

`vela module deploy` does not send a placement. Apply an Application of
`type: module`, as in `application.yaml`.

## Enabling the feature

`EnableModuleComponent` is off by default:

```shell
helm upgrade --install vela-core kubevela/vela-core \
  -n vela-system --create-namespace \
  --set featureGates.enableModuleComponent=true
```

## The module

`store/` is a publishable module. The class of each object is decided in this
order:

1. An annotation `resource.oam.dev/placement` already on the object. `hub` or
   `runtime`. This wins even for a file inside `auxiliary/hub/`.
2. A file directly in `auxiliary/hub/` or `<line>/auxiliary/hub/`, when the
   object has no annotation of its own. The folder is not walked into
   subdirectories. Any other directory under `auxiliary/` is ignored.
3. `placement` on the line, in `<line>/_version.cue`.
4. `placement` on the module, in `_module.cue`.
5. Nothing set. That is hub.

A value other than `hub` or `runtime`, including an empty string, is rejected
at publish. Definitions always install on the hub. A placement annotation on a
definition is accepted and then ignored for class.

In this module, `placement: "runtime"` is set on the module and on the `v1`
line, so unmarked objects follow the installer. `auxiliary/hub/admission.yaml`
has no annotation, so the folder makes it hub. `auxiliary/hub/keep-runtime.yaml`
carries `runtime`, so the annotation wins and publish warns.

```shell
vela module registry add modules <oci-registry-url> --type oci
vela module publish store --registry modules
```

## The installer

`application.yaml` asks for the runtime objects on the cluster named `spoke`.
Set `clusters` or `clusterLabelSelector`, not both. An empty
`clusterLabelSelector: {}` matches every cluster. Omit `placement` and the
render stays flat: every object installs on the hub, which is the same result
as marking them hub.

```shell
kubectl apply -f application.yaml
```

The Application you applied owns `module-store` in `vela-system`. That owned
Application has two deploy steps, `deploy-module-hub` and
`deploy-module-runtime`. The hub step selects the local cluster. The runtime
step selects the clusters from `placement`.

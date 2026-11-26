# Crate placement

Install a crate from an Application and choose which clusters receive the
objects marked runtime. Definitions, and anything marked hub, stay on the hub
cluster. The hub is the cluster where KubeVela itself runs.

You can set the placement in two ways. Apply an Application of
`type: crate`, as in `application.yaml`, or pass `--clusters` or
`--cluster-selector` to `vela crate deploy`. Without either flag,
`vela crate deploy` sends no placement.

## Enabling the feature

`EnableCrateComponent` is off by default:

```shell
helm upgrade --install vela-core kubevela/vela-core \
  -n vela-system --create-namespace \
  --set featureGates.enableCrateComponent=true
```

## The crate

`store/` is a publishable crate. The class of each object is decided in this
order:

1. An annotation `resource.oam.dev/placement` already on the object. `hub` or
   `runtime`. This wins even for a file inside `auxiliary/hub/`.
2. A file directly in `auxiliary/hub/` or `<line>/auxiliary/hub/`, when the
   object has no annotation of its own. The folder is not walked into
   subdirectories. Any other directory under `auxiliary/` is ignored.
3. `placement` on the line, in `<line>/_version.cue`.
4. `placement` on the crate, in `_crate.cue`.
5. Nothing set. That is hub.

A value other than `hub` or `runtime`, including an empty string, is rejected
at publish. Definitions always install on the hub. A placement annotation on a
definition is accepted and then ignored for class.

In this crate, `placement: "runtime"` is set on the crate and on the `v1`
line, so unmarked objects follow the installer. `auxiliary/hub/admission.yaml`
has no annotation, so the folder makes it hub. `auxiliary/hub/keep-runtime.yaml`
carries `runtime`, so the annotation wins and publish warns.

`v1/definitions/bucket.cue` is the v1 capability. It has no placement
annotation, and it still installs on the hub. Reference it as `store/v1/bucket`.
`v1/definitions/note.yaml` is a definition that does carry `runtime`. The
annotation is kept on the object and ignored for class, so it installs on the
hub with `bucket`.

```shell
vela crate registry add crates <oci-registry-url> --type oci
vela crate publish store --registry crates
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

The Application you applied owns `crate-store` in `vela-system`. That owned
Application has two deploy steps, `deploy-crate-hub` and
`deploy-crate-runtime`. The hub step selects the local cluster. The runtime
step selects the clusters from `placement`.

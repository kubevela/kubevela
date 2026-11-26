 Name | Description | Type | Required | Default | Immutable 
 ---- | ----------- | ---- | -------- | ------- | --------- 
 crate | Crate name. Must match `crate` in the package `_crate.cue`. | string | false | the component name |  
 registry | Name from `vela crate registry add`. Admission rejects a name that is not configured: `is not a configured crate registry; configured: [...]`. If admission cannot read the registry ConfigMap, it does not check the name. The workflow step then fails with `crate registry "<name>" not found` for a name that is not configured, or with the read error when the ConfigMap still cannot be read. Empty is resolved when KubeVela fetches the package: the only configured registry, or the registry named `catalog` when several exist. | string | false |  |  
 version | Package version (OCI tag). Empty selects the highest published semver tag when the workflow of the Application runs. A new tag in the registry does not upgrade the installed package, so set `version` on each install and each upgrade. This is not the API line. | string | false |  |  
 placement | Clusters that receive the objects that the crate marks `runtime`. Without `placement`, all objects install on the hub. See [placement](#placement-crate). | [placement](#placement-crate) | false |  |  

`namespace` is not a property. If you set it, admission rejects the Application. The definitions install into the namespace of this Application. That namespace must exist. On a managed cluster, a runtime object with no namespace also goes into this namespace, and the namespace must exist there too. KubeVela does not create it.

#### placement (crate)

Set `clusters` or `clusterLabelSelector`. Do not set both. See [Crate Multi-Cluster Placement](../../platform-engineers/crate/crate-multicluster.md).

 Name | Description | Type | Required | Default | Immutable 
 ---- | ----------- | ---- | -------- | ------- | --------- 
 clusters | Names of the clusters, as `vela cluster list` shows them. The list must contain one or more names. `local` is the hub. | []string | false |  |  
 clusterLabelSelector | Cluster labels to match. All labels must match. An empty map `{}` matches every cluster, including `local`. | map[string]string | false |  |  
 allowEmpty | If true, the install succeeds when `clusterLabelSelector` matches no cluster. If false, the runtime step fails. A name in `clusters` that is not a cluster always fails. | bool | false | false |  


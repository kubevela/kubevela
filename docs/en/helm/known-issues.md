# Limitations and known issues

Here are the highlights for using Helm Chart as schematic module. 

## Only one main workload in the Chart

The Chart must have exactly one workload considered as the main workload, e.g.,
Deployment, CloneSet, etc. 
In our context, `main workload` means the workload that will be tracked by
KubeVela controllers, applied with Traits and added into Scopes. 

To tell KubeVela which one is the main workload, two tasks are required:

#### 1. Declare main workload's resource definition

The field, `.spec.definitionRef` , in `WorkloadDefinition` is used to record the
resource definition of the main workload. 
The name should be in the format: `<resource>.<group>`. 

```yaml
apiVersion: core.oam.dev/v1alpha2
kind: WorkloadDefinition
...
spec:
  definitionRef:
    name: deployments.apps
    version: v1
```
```yaml
...
spec:
  definitionRef:
    name: clonesets.apps.kruise.io
    version: v1alpha1
```

#### 2. Qualified full name of the main workload

The name of the main workload should be templated with `qualifiied full name` by
default.
You can learn how Helm generate a qualifiied full name by `helm create` and
check [the helper template file](https://github.com/oam-dev/kubevela/blob/6c0141a62d950feed33cca69889d41fd55ece0a0/charts/vela-core/templates/_helpers.tpl#L10). 
Helm highly recommended that new charts are created via helm create command as 
the template names are automatically defined as per this best practice.
You must let your main workload use the templated full name as its name,
meanwhile NOT assign any value to `.Values.fullnameOverride`.

## Support OpenAPI v3 JSON schema  

Since uses can override default values of a Chart through Application's
`settings`, we should help users to be familiar with the content of a Helm
Chart's `Values.yaml`.
Currently users can only learn it from reading Chart's README doc or 
`Values.yaml` file.
We will integerate with the [openapi-v3-json-schema automatically generation](https://kubevela.io/#/en/platform-engineers/openapi-v3-json-schema.md) soon.

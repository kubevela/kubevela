# Limitations and known issues

Here are the highlights for using Helm Chart as schematic module. 

## Only one main workload in the Chart

The Chart must have exactly one workload being regarded as the **main**
workload.
In our context, `main workload` means the workload that will be tracked by
KubeVela controllers, applied with traits and added into scopes. 
For example, the `main workload` will benifit from KubeVela with unified
rollout, revision, traffic management, etc.

To tell KubeVela which one is the main workload, you must follow these two steps:

#### 1. Declare main workload's resource definition

The field `.spec.definitionRef` in `WorkloadDefinition` is used to record the
resource definition of the main workload. 
The name should be in the format: `<resource>.<group>`. 
 
For example, the Deployment resource should be defined as:
```yaml
apiVersion: core.oam.dev/v1alpha2
kind: WorkloadDefinition
...
spec:
  definitionRef:
    name: deployments.apps
    version: v1
```
The clonset resource should be defined as:
```yaml
...
spec:
  definitionRef:
    name: clonesets.apps.kruise.io
    version: v1alpha1
```

#### 2. Qualified full name of the main workload

The name of the main workload should be templated with [a default fully
qualified app
name](https://github.com/helm/helm/blob/543364fba59b0c7c30e38ebe0f73680db895abb6/pkg/chartutil/create.go#L415).
Helm is highly recommended that new charts are created via `helm create` command
as the template names are automatically defined as per this best practice.  You
must let your main workload use the templated full name as its name.
DO NOT assign any value to `.Values.fullnameOverride`.

## Upgrade an application

By contrast to CUE based workload, application using Helm schematic workload
cannot benefit from [application level rollout](https://github.com/oam-dev/kubevela/blob/master/design/vela-core/rollout-design.md#applicationdeployment-workflow) because they use totally different mechanisms to create and 
manage workloads.
So currently [application inplace upgrade](https://github.com/oam-dev/kubevela/blob/master/design/vela-core/rollout-design.md#application-inplace-upgrade-workflow) is the only one choice for users who
want to upgrade their applications containing Helm schematic workloads.
Just as [the sample](./trait.md#update-an-applicatiion) shows, users can modify
the application's configuration directly to upgrade it.

#### Changing settings will trigger Helm release upgrade

For Helm schematic workload, `.spec.components.settings` in the config of
application will override the default values of a Chart.
Any changes applied to `settings` will trigger a Helm release upgrade.
Most of this procedure is handled by Helm and Flux2/helm-controller,
while KubeVela has no opinion on it, even upgrade has failed or timeout.

On one hand, users should be very clear with what will happen after making such
an upgrade to the Chart release. On the other hand, users can define remediation
strategies in the Helm schematic according to [fluxcd/helmrelease API
doc](https://github.com/fluxcd/helm-controller/blob/main/docs/api/helmrelease.md#upgraderemediation)
and [spec doc](https://toolkit.fluxcd.io/components/helm/helmreleases/#configuring-failure-remediation) 
in case of upgrade failure.

For example
```yaml
apiVersion: core.oam.dev/v1alpha2
kind: WorkloadDefinition
metadata:
  name: webapp-chart
spec:
...
  schematic:
    helm:
      release:
        chart:
          spec:
            chart: "podinfo"
            version: "5.1.4"
        upgrade:
          remediation:
            retries: 3 
            remediationStrategy: rollback
      repository:
        url: "http://oam.dev/catalog/"

```

Currently, users can hardly get helpful information of a living Helm release to
figure out what happened if upgrading failed.  
We will enhance the observability to help users track the situation of Helm
release in application level.

#### Changing traits may make Pod restart

Traits work on Helm schematic workload in the same way as CUE based workload.
The changes on traits will effect the main workload finally.
Users should pay attention that, pods belonging to the workload may restart
because of various possible changes applied to the workload, that will result 
in service down.

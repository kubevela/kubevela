# Limitations and Known Issues

Here are some known issues for using Helm chart as application component. Pleas note most of these restrictions will be fixed over time.

## Only one main workload in the chart

The chart must have exactly one workload being regarded as the **main** workload. In this context, `main workload` means the workload that will be tracked by KubeVela controllers, applied with traits and added into scopes. Only the `main workload` will benefit from KubeVela with rollout, revision, traffic management, etc.

To tell KubeVela which one is the main workload, you must follow these two steps:

#### 1. Declare main workload's resource definition

The field `.spec.definitionRef` in `WorkloadDefinition` is used to record the
resource definition of the main workload. 
The name should be in the format: `<resource>.<group>`. 
 
For example, the Deployment resource should be defined as:
```yaml
apiVersion: core.oam.dev/v1beta1
kind: ComponentDefinition
...
spec:
  workload:
    definition:
      apiVersion: apps/v1
      kind: Deployment
```
The CloneSet workload resource should be defined as:
```yaml
...
spec:
  workload:
    definition:
      apiVersion: apps.kruise.io/v1alpha1
      kind: Cloneset
```

#### 2. Qualified full name of the main workload

The name of the main workload should be templated with [a default fully
qualified app
name](https://github.com/helm/helm/blob/543364fba59b0c7c30e38ebe0f73680db895abb6/pkg/chartutil/create.go#L415). DO NOT assign any value to `.Values.fullnameOverride`.

> Also, Helm highly recommend that new charts are created via `$ helm create` command so the template names are automatically defined as per this best practice.

## Upgrade the application

#### Rollout strategy

For now, Helm based components cannot benefit from [application level rollout strategy](https://github.com/oam-dev/kubevela/blob/master/design/vela-core/rollout-design.md#applicationdeployment-workflow).

So currently in-place upgrade by modifying the application specification directly is the only way to upgrade the Helm based components, no advanced rollout strategy can be assigned to it. Please check [this sample](./trait.md#update-an-applicatiion).

#### Changing `settings` will trigger Helm release upgrade

For Helm based component, `.spec.components.settings` is the way user override the default values of the chart, so any changes applied to `settings` will trigger a Helm release upgrade.

This process is handled by Helm and `Flux2/helm-controller`, hence you can define remediation
strategies in the schematic according to [fluxcd/helmrelease API
doc](https://github.com/fluxcd/helm-controller/blob/main/docs/api/helmrelease.md#upgraderemediation)
and [spec doc](https://toolkit.fluxcd.io/components/helm/helmreleases/#configuring-failure-remediation) 
in case failure happens during this upgrade.

For example
```yaml
apiVersion: core.oam.dev/v1beta1
kind: ComponentDefinition
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

> Note: currently, it's hard to get helpful information of a living Helm release to figure out what happened if upgrading failed. We will enhance the observability to help users track the situation of Helm release in application level.

#### Changing `traits` may make Pods restart

Traits work on Helm based component in the same way as CUE based component, i.e. changes on traits may impact the main workload instance. Hence, the Pods belonging to this workload instance may restart twice during upgrade, one is by the Helm upgrade, and the other one is caused by traits.

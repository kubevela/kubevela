---
title:  Known Limitations
---

## Limitations

Here are some known limitations for using Helm chart as application component.

### Workload Type Indicator

Following best practices of microservice, KubeVela recommends only one workload resource present in one Helm chart. Please split your "super" Helm chart into multiple charts (i.e. components). Essentially, KubeVela relies on the `workload` filed in component definition to indicate the workload type it needs to take care, for example:

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
```yaml
...
spec:
  workload:
    definition:
      apiVersion: apps.kruise.io/v1alpha1
      kind: Cloneset
```

 Note that KubeVela won't fail if multiple workload types are packaged in one chart, the issue is for further operational behaviors such as rollout, revisions, and traffic management, they can only take effect on the indicated workload type.

### Always Use Full Qualified Name

The name of the workload should be templated with [fully qualified application name](https://github.com/helm/helm/blob/543364fba59b0c7c30e38ebe0f73680db895abb6/pkg/chartutil/create.go#L415) and please do NOT assign any value to `.Values.fullnameOverride`. As a best practice, Helm also highly recommend that new charts should be created via `$ helm create` command so the template names are automatically defined as per this best practice.

### Control the Application Upgrade

Changes made to the component `properties` will trigger a Helm release upgrade. This process is handled by Flux v2 Helm controller, hence you can define remediation
strategies in the schematic based on [Helm Release
documentation](https://github.com/fluxcd/helm-controller/blob/main/docs/api/helmrelease.md#upgraderemediation)
and [specification](https://toolkit.fluxcd.io/components/helm/helmreleases/#configuring-failure-remediation) 
in case failure happens during this upgrade.

For example:
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

Though one issue is for now it's hard to get helpful information of a living Helm release to figure out what happened if upgrading failed. We will enhance the observability to help users track the situation of Helm release in application level.

## Issues

The known issues will be fixed in following releases.

### Rollout Strategy

For now, Helm based components cannot benefit from [application level rollout strategy](https://github.com/oam-dev/kubevela/blob/master/design/vela-core/rollout-design.md#applicationdeployment-workflow). As shown in [this sample](./trait#update-an-applicatiion), if the application is updated, it can only be rollouted directly without canary or blue-green approach.

### Updating Traits Properties may Also Lead to Pods Restart

Changes on traits properties may impact the component instance and Pods belonging to this workload instance will restart. In CUE based components this is avoidable as KubeVela has full control on the rendering process of the resources, though in Helm based components it's currently deferred to Flux v2 controller.

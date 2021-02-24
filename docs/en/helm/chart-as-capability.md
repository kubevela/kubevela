# Use Helm chart as capability module

Here is an example of how to use Helm chart as workload capability module.

## Install fluxcd/flux2 as dependencies

This feature depends on several CRDs and controllers from [fluxcd/flux2](https://github.com/fluxcd/flux2), so we prepared a simplified Helm chart to install dependencies.

It's worth noting that, flux2 doesn't offer an official Helm chart to install.
And this chart only includes minimum dependencies this feature relies on, not all of flux2.

```shell
	helm install --create-namespace -n flux-system helm-flux http://oam.dev/catalog/helm-flux2-0.1.0.tgz
```

## Write WorkloadDefinition 
Here is an example `WorkloadDefinition` with only required data of a Helm module.

Comparing to existing workload definition based on CUE template, several points worth attention in Helm module.

- `.spec.definitionRef` is required to indicate the workload GVK in your Helm chart. For example, in our sample chart, the core workload is `deployments.apps/v1`.
- `.spec.schematic.helm` contains information of Helm release & repository.

```yaml
apiVersion: core.oam.dev/v1alpha2
kind: WorkloadDefinition
metadata:
  name: webapp-chart
  annotations:
    definition.oam.dev/description: helm chart for webapp
spec:
  definitionRef:
    name: deployments.apps
    version: v1
  schematic:
    helm:
      release:
        chart:
          spec:
            chart: "podinfo"
            version: "5.1.4"
      repository:
        url: "http://oam.dev/catalog/"
```
Specifically, the definition follows the APIs from `fluxcd/flux2`, [HelmReleaseSpec](https://github.com/fluxcd/helm-controller/blob/main/docs/api/helmrelease.md) and [HelmRepositorySpec](https://github.com/fluxcd/source-controller/blob/main/docs/api/source.md#source.toolkit.fluxcd.io/v1beta1.HelmRepository).
However, the fields shown in the sample are almost enough to describe a Helm chart release and its repository.

## Define Application & Deploy

Here is an example `Application`.

```yaml
apiVersion: core.oam.dev/v1alpha2
kind: Application
metadata:
  name: myapp
  namespace: default
spec:
  components:
    - name: demo-podinfo 
      type: webapp-chart 
      settings: 
        image:
          tag: "5.1.2"
```
Helm module workload will use data in `settings` as [Helm chart values](https://github.com/captainroy-hy/podinfo/blob/master/charts/podinfo/values.yaml).
You can read the README.md of the Helm chart, and the arguments are totally align with [values.yaml](https://github.com/captainroy-hy/podinfo/blob/master/charts/podinfo/values.yaml) of the chart.

Now we can deploy the application.

```shell
kubectl apply -f webapp-chart-wd.yaml 

kubectl apply -f myapp.yaml
```

After several minutes (it takes time to fetch Helm chart from the repo, render and install), you can check the Helm release is installed.
```shell
helm ls -A

myapp-demo-podinfo	default  	1 	2021-03-05 02:02:18.692317102 +0000 UTC	deployed	podinfo-5.1.4   	5.1.4
```
And check the deployment defined in the chart.
```shell
kubectl get deploy

NAME                     READY   UP-TO-DATE   AVAILABLE   AGE
myapp-demo-podinfo   1/1     1            1           66m
```
## Use existing Trait system

A Helm module workload can fully work with Traits in the same way as existing workloads. 
For example, we add two exemplary traits, scaler and [virtualgroup](https://github.com/oam-dev/kubevela/blob/master/docs/examples/helm-module/virtual-group-td.yaml), to a Helm module workload.

```yaml
apiVersion: core.oam.dev/v1alpha2
kind: Application
metadata:
  name: myapp
  namespace: default
spec:
  components:
    - name: demo-podinfo 
      type: webapp-chart 
      settings: 
        image:
          tag: "5.1.2"
      traits:
        - name: scaler
          properties:
            replicas: 4
        - name: virtualgroup
          properties:
            group: "my-group1"
            type: "cluster"
```
> If vela webhook is enabled, remember to add `deployments.apps` into the trait definition's `.spec.appliesToWorkloads` list

:exclamation: Only one thing you should pay attention when use Trait system with Helm module workload, **make sure the target workload in your Helm chart strictly follows the qualified-full-name convention in Helm.**
[As the sample chart shows](https://github.com/captainroy-hy/podinfo/blob/c2b9603036f1f033ec2534ca0edee8eff8f5b335/charts/podinfo/templates/deployment.yaml#L4), the workload name is composed of [release name and chart name](https://github.com/captainroy-hy/podinfo/blob/c2b9603036f1f033ec2534ca0edee8eff8f5b335/charts/podinfo/templates/_helpers.tpl#L13). 
KubeVela will generate a release name based on your Application name and component name automatically, so you just make sure not overried the full name template in your Helm chart. 

KubeVela relies on the name to discovery the workload, otherwise it cannot apply traits to the workload.

### Verify applications with traits

You may wait a bit more time to check the trait works after deploying the application. 
Because KubeVela may not discovery the target workload immediately when it's created because of reconciliation interval.

Check the scaler trait.
```shell
kubectl get manualscalertrait

NAME                            AGE
demo-podinfo-scaler-d8f78c6fc   13m
```

Check the virtualgroup trait.
```shell
kubectl get deployment myapp-demo-podinfo -o json | jq .spec.template.metadata.labels

{
  "app.cluster.virtual.group": "my-group1",
  "app.kubernetes.io/name": "myapp-demo-podinfo"
}
```

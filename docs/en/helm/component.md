# Use Helm To Define a Component

This documentation explains how to use Helm chart to define an application component.

## Install fluxcd/flux2 as dependencies

Using helm as a workload depends on several CRDs and controllers from [fluxcd/flux2](https://github.com/fluxcd/flux2), make sure you have make them installed before continue.

It's worth to note that flux2 doesn't offer an official Helm chart to install,
so we provide a chart which only includes minimal dependencies KubeVela relies on as an alternative choice.

Install the minimal flux2 chart provided by KubeVela:
```shell
$ helm install --create-namespace -n flux-system helm-flux http://oam.dev/catalog/helm-flux2-0.1.0.tgz
```

## Write WorkloadDefinition 
Here is an example `WorkloadDefinition` about how to use Helm as schematic module.

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

Just like using CUE as schematic module, we also have some rules and contracts to use helm chart as schematic module.

- `.spec.definitionRef` is required to indicate the main workload(Group/Verison/Kind) in your Helm chart.
Only one workload allowed in one helm chart.
For example, in our sample chart, the core workload is `deployments.apps/v1`, other resources will also be deployed but mechanism of KubeVela won't work for them.
- `.spec.schematic.helm` contains information of Helm release & repository.

There are two fields `release` and `repository` in the `.spec.schematic.helm` section, these two fields align with the APIs of `fluxcd/flux2`. Spec of `release` aligns with [`HelmReleaseSpec`](https://github.com/fluxcd/helm-controller/blob/main/docs/api/helmrelease.md) and spec of `repository` aligns with [`HelmRepositorySpec`](https://github.com/fluxcd/source-controller/blob/main/docs/api/source.md#source.toolkit.fluxcd.io/v1beta1.HelmRepository).
In a word, just like the fields shown in the sample, the helm schematic module describes a specific Helm chart release and its repository.

## Create an Application using the helm based WorkloadDefinition

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
You can learn the schema of settings by reading the `README.md` of the Helm
chart, and the schema are totally align with
[`values.yaml`](https://github.com/captainroy-hy/podinfo/blob/master/charts/podinfo/values.yaml)
of the chart.  

Helm v3 has [support to validate
values](https://helm.sh/docs/topics/charts/#schema-files) in a chart's
values.yaml file with JSON schemas.  
Vela will try to fetch the `values.schema.json` file from the Chart archive and
[save the schema into a
ConfigMap](https://kubevela.io/#/en/platform-engineers/openapi-v3-json-schema.md)
which can be consumed latter through UI or CLI.  
If `values.schema.json` is not provided by the Chart author, Vela will generate a
OpenAPI-v3 JSON schema based on the `values.yaml` file automatically.  

Deploy the application and after several minutes (it takes time to fetch Helm chart from the repo, render and install), you can check the Helm release is installed.
```shell
$ helm ls -A
myapp-demo-podinfo	default  	1 	2021-03-05 02:02:18.692317102 +0000 UTC	deployed	podinfo-5.1.4   	5.1.4
```
Check the deployment defined in the chart has been created successfully.
```shell
$ kubectl get deploy
NAME                     READY   UP-TO-DATE   AVAILABLE   AGE
myapp-demo-podinfo   1/1     1            1           66m
```

Check the values(`image.tag = 5.1.2`) from application's `settings` are assigned to the chart.
```shell
$ kubectl get deployment myapp-demo-podinfo -o json | jq '.spec.template.spec.containers[0].image'
"ghcr.io/stefanprodan/podinfo:5.1.2"
```

---
title:  How-to
---

In this section, it will introduce how to declare Helm charts as app components via `ComponentDefinition`.

> Before reading this part, please make sure you've learned [the definition and template concepts](../platform-engineers/definition-and-templates).

## Prerequisite

* Make sure you have enabled Helm support in the [installation guide](/docs/install).

## Declare `ComponentDefinition`

Here is an example `ComponentDefinition` about how to use Helm as schematic module.

```yaml
apiVersion: core.oam.dev/v1beta1
kind: ComponentDefinition
metadata:
  name: webapp-chart
  annotations:
    definition.oam.dev/description: helm chart for webapp
spec:
  workload:
    definition:
      apiVersion: apps/v1
      kind: Deployment
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

In detail:
- `.spec.workload` is required to indicate the workload type of this Helm based component. Please also check for [Known Limitations](/docs/helm/known-issues?id=workload-type-indicator) if you have multiple workloads packaged in one chart.
- `.spec.schematic.helm` contains information of Helm `release` and `repository` which leverages `fluxcd/flux2`.
  - i.e. the pec of `release` aligns with [`HelmReleaseSpec`](https://github.com/fluxcd/helm-controller/blob/main/docs/api/helmrelease.md) and spec of `repository` aligns with [`HelmRepositorySpec`](https://github.com/fluxcd/source-controller/blob/main/docs/api/source.md#source.toolkit.fluxcd.io/v1beta1.HelmRepository).

## Declare an `Application`

Here is an example `Application`.

```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: myapp
  namespace: default
spec:
  components:
    - name: demo-podinfo 
      type: webapp-chart 
      properties: 
        image:
          tag: "5.1.2"
```

The component `properties` is exactly the [overlay values](https://github.com/captainroy-hy/podinfo/blob/master/charts/podinfo/values.yaml) of the Helm chart.

Deploy the application and after several minutes (it may take time to fetch Helm chart), you can check the Helm release is installed.
```shell
$ helm ls -A
myapp-demo-podinfo  default   1   2021-03-05 02:02:18.692317102 +0000 UTC deployed  podinfo-5.1.4     5.1.4
```
Check the workload defined in the chart has been created successfully.
```shell
$ kubectl get deploy
NAME                     READY   UP-TO-DATE   AVAILABLE   AGE
myapp-demo-podinfo   1/1     1            1           66m
```

Check the values (`image.tag = 5.1.2`) from application's `properties` are assigned to the chart.
```shell
$ kubectl get deployment myapp-demo-podinfo -o json | jq '.spec.template.spec.containers[0].image'
"ghcr.io/stefanprodan/podinfo:5.1.2"
```


### Generate Form from Helm Based Components

KubeVela will automatically generate OpenAPI v3 JSON schema based on [`values.schema.json`](https://helm.sh/docs/topics/charts/#schema-files) in the Helm chart, and store it in a `ConfigMap` in the same `namespace` with the definition object. Furthermore, if `values.schema.json` is not provided by the chart author, KubeVela will generate OpenAPI v3 JSON schema based on its `values.yaml` file automatically. 

Please check the [Generate Forms from Definitions](/docs/platform-engineers/openapi-v3-json-schema) guide for more detail of using this schema to render GUI forms. 

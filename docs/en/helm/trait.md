# Use existing Trait system

A Helm module workload can fully work with traits in the same way as the CUE based schematic.
In this sample application, we add two exemplary traits,
[scaler](https://github.com/oam-dev/kubevela/blob/master/charts/vela-core/templates/defwithtemplate/manualscale.yaml)
and
[virtualgroup](https://github.com/oam-dev/kubevela/blob/master/docs/examples/helm-module/virtual-group-td.yaml),
to a Helm module workload.

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

:exclamation: Only one thing you should pay attention when use Trait system with Helm module workload, **make sure the target workload in your Helm chart strictly follows the qualified-full-name convention in Helm.**
[As the sample chart shows](https://github.com/captainroy-hy/podinfo/blob/c2b9603036f1f033ec2534ca0edee8eff8f5b335/charts/podinfo/templates/deployment.yaml#L4), the workload name is composed of [release name and chart name](https://github.com/captainroy-hy/podinfo/blob/c2b9603036f1f033ec2534ca0edee8eff8f5b335/charts/podinfo/templates/_helpers.tpl#L13). 
KubeVela will generate a release name based on your Application name and component name automatically, so you just make sure not override the full name template in your Helm chart. 

KubeVela relies on the name to discovery the workload, otherwise it cannot apply traits to the workload.

## Verify traits work correctly

You may wait a bit more time to check the trait works after deploying the application. 
Because KubeVela may not discovery the target workload immediately when it's created because of reconciliation interval.

Check the scaler trait.
```shell
kubectl get manualscalertrait

NAME                            AGE
demo-podinfo-scaler-d8f78c6fc   13m
```
```shell
kubectl get deployment myapp-demo-podinfo -o json | jq .spec.replicas

4
```

Check the virtualgroup trait.
```shell
kubectl get deployment myapp-demo-podinfo -o json | jq .spec.template.metadata.labels

{
  "app.cluster.virtual.group": "my-group1",
  "app.kubernetes.io/name": "myapp-demo-podinfo"
}
```

## Update an application

After the application is deployed and workloads/traits are created successfully,
you can update the application, and corresponding changes will be applied to the
workload.

Let's make several changes on the configuration of the sample application.

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
          tag: "5.1.3" # 5.1.2 => 5.1.3 
      traits:
        - name: scaler
          properties:
            replicas: 2 # 4 => 2
        - name: virtualgroup
          properties:
            group: "my-group2" # my-group1 => my-group2
            type: "cluster"
```

Apply the new configuration and check the results after several minutes.

Check the new values(`image.tag = 5.1.3`) from application's `settings` are assigned to the chart.
```shell
kubectl get deployment myapp-demo-podinfo -o json | jq '.spec.template.spec.containers[0].image'

"ghcr.io/stefanprodan/podinfo:5.1.3"
```
Under the hood, Helm makes an upgrade to the release (revision 1 => 2).
```shell
helm ls -A

NAME              	NAMESPACE	REVISION	UPDATED                                	STATUS  	CHART        	APP VERSION
myapp-demo-podinfo	default  	2       	2021-03-15 08:52:00.037690148 +0000 UTC	deployed	podinfo-5.1.4	5.1.4
```

Check the scaler trait.
```shell
kubectl get deployment myapp-demo-podinfo -o json | jq .spec.replicas

2
```

Check the virtualgroup trait.
```shell
kubectl get deployment myapp-demo-podinfo -o json | jq .spec.template.metadata.labels

{
  "app.cluster.virtual.group": "my-group2",
  "app.kubernetes.io/name": "myapp-demo-podinfo"
}
```

## Delete a trait

Let's have a try removing a trait from the application.

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
          tag: "5.1.3"
      traits:
        # - name: scaler
        #   properties:
        #    replicas: 2 
        - name: virtualgroup
          properties:
            group: "my-group2"
            type: "cluster"
```

Apply the configuration and check `manualscalertrait` has been deleted.
```shell
kubectl get manualscalertrait

No resources found
```


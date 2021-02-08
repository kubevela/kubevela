# Status Loop Back

In the previous sections, we use CUE as template to render K8s resources. After an application deployed, the status
loop back can also use CUE.

KubeVela use CUE to define health check and custom status message for an application in workload type and trait.

## Health Check

The spec of health check is `spec.status.healthPolicy`, they are the same for both Workload Type and Trait.

If not defined, the health result will always be `true`.

The keyword in CUE is `isHealth`, the result of CUE expression must be `bool` type.
Application CRD controller will evaluate the CUE expression periodically until it
becomes healthy. Every time the controller will get all the K8s resources and fill them into the context field.

So the context will contain following information:

```cue
context:{
  name: <component name>
  appName: <app name>
  output: <K8s workload resource>
  outputs: {
    <resource1>: <K8s trait resource1>
    <resource2>: <K8s trait resource2>
  }
}
```

Trait will not have the `context.ouput`, other fields are the same.

The example of health check likes below:

```yaml
apiVersion: core.oam.dev/v1alpha2
kind: WorkloadDefinition
spec:
  status:
    healthPolicy: |
      isHealth: (context.output.status.readyReplicas > 0) && (context.output.status.readyReplicas == context.output.status.replicas)
   ...
```

```yaml
apiVersion: core.oam.dev/v1alpha2
kind: TraitDefinition
spec:
  status:
    healthPolicy: |
      isHealth: len(context.outputs.service.spec.clusterIP) > 0
   ...
```

Refer to [this doc](https://github.com/oam-dev/kubevela/blob/master/config/samples/app-with-status/template.yaml) for the complete example.

The health check result will be recorded into the Application CRD resource.

```yaml
apiVersion: core.oam.dev/v1alpha2
kind: Application
spec:
  components:
  - name: myweb
    settings:
      cmd:
      - sleep
      - "1000"
      enemies: alien
      image: busybox
      lives: "3"
    traits:
    - name: ingress
      properties:
        domain: www.example.com
        http:
          /: 80
    type: worker
status:
  ...
  services:
  - healthy: true
    message: "type: busybox,\t enemies:alien"
    name: myweb
    traits:
    - healthy: true
      message: 'Visiting URL: www.example.com, IP: 47.111.233.220'
      type: ingress
  status: running
```

## Custom Status

The spec of custom status is `spec.status.customStatus`, they are the same for both Workload Type and Trait.

The keyword in CUE is `message`, the result of CUE expression must be `string` type.

The custom status has the same mechanism with health check.
Application CRD controller will evaluate the CUE expression after the health check succeed.

The context will contain following information:

```cue
context:{
  name: <component name>
  appName: <app name>
  output: <K8s workload resource>
  outputs: {
    <resource1>: <K8s trait resource1>
    <resource2>: <K8s trait resource2>
  }
}
```

Trait will not have the `context.ouput`, other fields are the same.


Refer to [this doc](https://github.com/oam-dev/kubevela/blob/master/config/samples/app-with-status/template.yaml) for the complete example.

```yaml
apiVersion: core.oam.dev/v1alpha2
kind: WorkloadDefinition
spec:
  status:
    customStatus: |-
      message: "type: " + context.output.spec.template.spec.containers[0].image + ",\t enemies:" + context.outputs.gameconfig.data.enemies
   ...
```

```yaml
apiVersion: core.oam.dev/v1alpha2
kind: TraitDefinition
spec:
  status:
    customStatus: |-
      message: "type: "+ context.outputs.service.spec.type +",\t clusterIP:"+ context.outputs.service.spec.clusterIP+",\t ports:"+ "\(context.outputs.service.spec.ports[0].port)"+",\t domain"+context.outputs.ingress.spec.rules[0].host
   ...
```

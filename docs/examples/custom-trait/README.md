The Kubevela platform out of the box provides supported and complete `StatefulSet` component and `Storage` trait. This example is just provided for education.

Please check also the online documentation: 
* [Trait Definition](https://kubevela.io/docs/platform-engineers/traits/customize-trait/)
* [Component Definition](https://kubevela.io/docs/platform-engineers/components/custom-component/)

# Yet another example defining a custom component and custom trait

1. define a stateful component with StatefulSet as output

```shell
$ vela def apply stateful.cue
ComponentDefinition test-stateful created in namespace vela-system.
```

2. define a custom trait with patch volume

```shell
$ vela def apply volume-trait.cue
TraitDefinition storageclass created in namespace vela-system.
```

3. You can validate it by:
```
$ vela def vet volume-trait.cue 
Validation succeed.
```



4. try dry run your app:
```
vela dry-run -f app.yaml 
```

```yaml
# Application(website) -- Component(custom-component)
---

apiVersion: apps/v1
kind: StatefulSet
metadata:
  annotations: {}
  labels:
    app.oam.dev/appRevision: ""
    app.oam.dev/component: custom-component
    app.oam.dev/name: website
    app.oam.dev/namespace: default
    app.oam.dev/resourceType: WORKLOAD
    workload.oam.dev/type: test-stateful
  name: custom-component
  namespace: default
spec:
  minReadySeconds: 10
  replicas: 1
  selector:
    matchLabels:
      app: custom-component
  serviceName: custom-component
  template:
    metadata:
      labels:
        app: custom-component
    spec:
      containers:
      - image: nginx:latest
        name: nginx
        ports:
        - containerPort: 80
          name: web
        volumeMounts:
        - mountPath: /usr/share/nginx/html
          name: test
      terminationGracePeriodSeconds: 10
  volumeClaimTemplates:
  - metadata:
      name: test
    spec:
      accessModes:
      - ReadWriteOnce
      resources:
        requests:
          storage: 10Gi
      storageClassName: cbs

---
apiVersion: v1
kind: Service
metadata:
  annotations: {}
  labels:
    app: custom-component
    app.oam.dev/appRevision: ""
    app.oam.dev/component: custom-component
    app.oam.dev/name: website
    app.oam.dev/namespace: default
    app.oam.dev/resourceType: TRAIT
    trait.oam.dev/resource: web
    trait.oam.dev/type: AuxiliaryWorkload
  name: custom-component
  namespace: default
spec:
  clusterIP: None
  ports:
  - name: web
    port: 80
  selector:
    app: custom-component
```
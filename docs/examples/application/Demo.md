# Application Example

In this Demo, Application application-sample will be converted to appcontext and component

The fields in the application spec come from the parametes defined in the definition template
, so we must install Definition at first

Step 1: Install ComponentDefinition & Trait Definition
```
kubectl apply -f template.yaml
```
Step 2: Create a sample application in the cluster
```
kubectl apply -f application-sample.yaml
```
Step 3: View the application status
```
kubectl get -f application-sample.yaml -oyaml

// You can see the following
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  annotations:
    kubectl.kubernetes.io/last-applied-configuration: |
      {"apiVersion":"core.oam.dev/v1beta1","kind":"Application","metadata":{"annotations":{},"name":"application-sample","namespace":"default"},"spec":{"components":[{"name":"myweb","properties":{"cmd":["sleep","1000"],"image":"busybox"},"traits":[{"properties":{"replicas":10},"type":"scaler"},{"properties":{"image":"nginx","name":"sidecar-test"},"type":"sidecar"},{"properties":{"http":{"server":80}},"type":"kservice"}],"type":"worker"}]}}
  ...   
spec:
  components:
  - name: myweb
    properties:
      cmd:
      - sleep
      - "1000"
      image: busybox
    traits:
    - properties:
        replicas: 10
      type: scaler
    - properties:
        image: nginx
        name: sidecar-test
      type: sidecar
    - properties:
        http:
          server: 80
      type: kservice
    type: worker
status:
  batchRollingState: ""
  components:
  - apiVersion: core.oam.dev/v1alpha2
    kind: Component
    name: myweb
  conditions:
  - reason: Available
    status: "True"
    type: Parsed
  - reason: Available
    status: "True"
    type: Built
  - reason: Available
    status: "True"
    type: Applied
  status: running
  ...
```


```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: example
  namespace: default
spec:
  components:
    - name: express-server
      type: webservice
      properties:
        image: crccheck/hello-world
        port: 8000
    - name: express-server2
      type: webservice
      properties:
        image: crccheck/hello-world
        port: 8000

  workflow:
    steps:
      - name: step
        type: step-group
        subSteps:
          - name: apply-sub-step1
            type: apply-component
            properties:
              component: express-server
          - name: apply-sub-step2
            type: apply-component
            properties:
              component: express-server2
```

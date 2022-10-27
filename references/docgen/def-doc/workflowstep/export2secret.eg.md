```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: export-secret
  namespace: default
spec:
  components:
  - name: express-server-sec
    type: webservice
    properties:
      image: oamdev/hello-world
      port: 8000
  workflow:
    steps:
      - name: apply-server
        type: apply-component
        outputs: 
          - name: status
            valueFrom: output.status.conditions[0].message
        properties:
          component: express-server-sec
      - name: export-secret
        type: export2secret
        inputs:
          - from: status
            parameterKey: data.serverstatus
        properties:
          secretName: my-secret
          data:
            testkey: |
              testvalue
              value-line-2
```
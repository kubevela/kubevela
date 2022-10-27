```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: export2config
  namespace: default
spec:
  components:
    - name: export2config-demo-server
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
          component: export2config-demo-server
      - name: export-config
        type: export2config
        inputs:
          - from: status
            parameterKey: data.serverstatus
        properties:
          configName: my-configmap
          data:
            testkey: |
              testvalue
              value-line-2
```
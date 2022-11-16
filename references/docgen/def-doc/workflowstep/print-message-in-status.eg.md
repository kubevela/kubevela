```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: print-message-in-status
  namespace: default
spec:
  components:
  - name: express-server
    type: webservice
    properties:
      image: oamdev/hello-world
      port: 8000
  workflow:
    steps:
      - name: express-server
        type: apply-component
        properties:
          component: express-server
      - name: message
        type: print-message-in-status
        properties:
          message: "All addons have been enabled successfully, you can use 'vela addon list' to check them."
```
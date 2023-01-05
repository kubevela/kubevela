```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: apply-deploy
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
      - name: apply-comp
        type: apply-component
        properties:
          component: express-server
      - name: apply-deploy
        type: apply-deployment
        properties:
          image: nginx
```
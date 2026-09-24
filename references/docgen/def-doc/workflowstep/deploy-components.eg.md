```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: deploy-components-example
  namespace: examples
spec:
  components:
    - name: web-on-local
      type: webservice
      properties:
        image: nginx
    - name: web-on-worker
      type: webservice
      properties:
        image: nginx
  policies:
    - name: topology-local
      type: topology
      properties:
        clusters: ["local"]
    - name: topology-worker
      type: topology
      properties:
        clusters: ["cluster-worker"]
  workflow:
    steps:
      - name: deploy-components
        type: deploy-components
        properties:
          components:
            - name: web-on-local
              policies: ["topology-local"]
            - name: web-on-worker
              policies: ["topology-worker"]
```

```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: example-app-rollout
  namespace: default
spec:
  components:
    - name: hello-world-server
      type: webservice
      properties:
        image: crccheck/hello-world
        ports: 
        - port: 8000
          expose: true
        type: webservice
  policies:
    - name: health-policy-demo
      type: health
      properties:
        probeInterval: 5
        probeTimeout: 10
```
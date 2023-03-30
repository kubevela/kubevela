```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: canary-demo
  annotations:
    app.oam.dev/publishVersion: v2
spec:
  components:
    - name: canary-demo
      type: webservice
      properties:
        image: wangyikewyk/canarydemo:v2
        ports:
          - port: 8090
      traits:
        - type: scaler
          properties:
            replicas: 5
        - type: gateway
          properties:
            domain: canary-demo.com
            http:
              "/version": 8090
  workflow:
    steps:
      - name: 200-status-percent-2-phase
        type: check-metrics
        timeout: 3m
        properties:
          query: sum(irate(nginx_ingress_controller_requests{host="canary-demo.com",status="200"}[5m]))/sum(irate(nginx_ingress_controller_requests{host="canary-demo.com"}[2m]))
          promAddress: "http://prometheus-server.o11y-system.svc:9090"
          condition: ">=0.95"
          duration: 2m
```

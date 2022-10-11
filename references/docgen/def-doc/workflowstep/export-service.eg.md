```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: app-collect-service-endpoint-and-export
spec:
  components:
    - type: webservice
      name: busybox
      properties:
        image: busybox
        imagePullPolicy: IfNotPresent
        cmd:
          - sleep
          - '1000000'
      traits:
        - type: expose
          properties:
            port: [8080]
            type: LoadBalancer
  policies:
    - type: topology
      name: local
      properties:
        clusters: ["local"]
    - type: topology
      name: worker
      properties:
        clusters: ["cluster-worker"]
  workflow:
    steps:
      - type: deploy
        name: deploy
        properties:
          policies: ["local"]
      - type: collect-service-endpoints
        name: collect-service-endpoints
        outputs:
          - name: host
            valueFrom: value.endpoint.host
          - name: port
            valueFrom: value.endpoint.port
      - type: export-service
        name: export-service
        properties:
          name: busybox
          topology: worker
        inputs:
          - from: host
            parameterKey: ip
          - from: port
            parameterKey: port
```
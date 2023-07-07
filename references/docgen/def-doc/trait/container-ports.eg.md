```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: busybox
spec:
  components:
  - name: busybox
    properties:
      cpu: "0.5"
      exposeType: ClusterIP
      image: busybox
      memory: 1024Mi
      ports:
      - expose: false
        port: 80
        protocol: TCP
      - expose: false
        port: 801
        protocol: TCP
    traits:
    - properties:
        containerName: busybox
        ports:
        - containerPort: 8080
          hostPort: 8080
          protocol: TCP
      type: container-ports
    type: webservice
```

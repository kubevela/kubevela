```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: busybox
spec:
  components:
    - name: busybox
      type: webservice
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
        - type: container-ports
          properties:
            # if you want to expose on the host and bind the external port to host
            # instead of Service(such as ClusterIP, NodePort, LoadBalancer and ExternalName),
            # you can use this trait to specify hostPort and hostIP.
            # you can use container-ports to control multiple containers by filling `containers`
            # NOTE: in containers, you must set the container name for each container
            containers:
              - containerName: busybox
                ports:
                  - containerPort: 80
                    protocol: TCP
                    hostPort: 8080
```

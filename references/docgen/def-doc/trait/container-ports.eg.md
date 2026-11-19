It's used to define Pod networks directly. hostPort routes the container's port directly to the port on the scheduled node, so that you can access the Pod through the host's IP plus hostPort.
Don't specify a hostPort for a Pod unless it is absolutely necessary(run `DaemonSet` service). When you bind a Pod to a hostPort, it limits the number of places the Pod can be scheduled, because each `<hostIP, hostPort, protocol>` combination must be unique. If you don't specify the hostIP and protocol explicitly, Kubernetes will use 0.0.0.0 as the default hostIP and TCP as the default protocol.
If you explicitly need to expose a Pod's port on the node, consider using `expose` or `gateway` trait, or exposeType and ports parameter of `webservice` component before resorting to `container-ports` trait.
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
            # you can use container-ports to control multiple containers by filling `containers`
            # NOTE: in containers, you must set the container name for each container
            containers:
              - containerName: busybox
                ports:
                  - containerPort: 80
                    protocol: TCP
                    hostPort: 8080
```

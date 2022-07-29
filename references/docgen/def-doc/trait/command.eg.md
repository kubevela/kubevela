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
        image: busybox
        cmd: ["sleep", "86400"]
      traits:
        - type: sidecar
          properties:
            name: sidecar-nginx
            image: nginx
        - type: command
          properties:
            # you can use command to control multiple containers by filling `containers`
            # NOTE: in containers, you must set the container name for each container
            containers:
              - containerName: busybox
                command: ["sleep", "8640000"]
              - containerName: sidecar-nginx
                args: ["-q"]
```
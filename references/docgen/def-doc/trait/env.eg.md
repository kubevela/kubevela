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
        - type: env
          properties:
            # you can use env to control multiple containers by filling `containers`
            # NOTE: in containers, you must set the container name for each container
            containers:
              - containerName: busybox
                env:
                  key_for_busybox_first: value_first
                  key_for_busybox_second: value_second
              - containerName: sidecar-nginx
                env:
                  key_for_nginx_first: value_first
                  key_for_nginx_second: value_second
```

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
        - type: env
          properties:
            # you can use env to control one container, if containerName not specified, it will patch on the first index container 
            containerName: busybox
            env:
              key_for_busybox_first: value_first
              key_for_busybox_second: value_second
```
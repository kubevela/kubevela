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
        - type: init-container
          properties:
            name: init-busybox
            image: busybox
            cmd: ["echo", "hello"]
            initMountPath: /data
            appMountPath: /data-initialized
```

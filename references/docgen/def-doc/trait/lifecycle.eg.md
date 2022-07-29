```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: application-with-lifecycle
spec:
  components:
    - name: busybox-runner
      type: worker
      properties:
        image: busybox
        cmd:
          - sleep
          - '1000'
      traits:
        - type: lifecycle
          properties:
            postStart:
              exec:
                command:
                  - echo
                  - 'hello world'
            preStop:
              httpGet:
                host: "www.aliyun.com"
                scheme: "HTTPS"
                port: 443
```
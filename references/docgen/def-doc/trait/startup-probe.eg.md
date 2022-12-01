```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: application-with-startup-probe
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
      - type: sidecar
        properties:
          name: nginx
          image: nginx
      # This startup-probe is blocking the startup of the main container 
      # as the URL has a typo '.comm' vs '.com'
      - type: startup-probe
        properties:
          containerName: "busybox-runner"
          httpGet:
            host: "www.guidewire.comm"
            scheme: "HTTPS"
            port: 443
          periodSeconds: 4
          failureThreshold: 4  
      # This startup probe targets the nginx sidecar
      - type: startup-probe
        properties:
          containerName: nginx
          httpGet:
            host: "www.guidewire.com"
            scheme: "HTTPS"
            port: 443
          periodSeconds: 5
          failureThreshold: 5           
```
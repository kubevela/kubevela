```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: podtato-head
spec:
  components:
    - name: podtato-head-frontend
      type: webservice
      properties:
        image: ghcr.io/podtato-head/podtato-server:v0.3.1
        ports:
          - port: 8080
            expose: true
        cpu: "0.1"
        memory: "32Mi"
      traits:
        - type: securitycontext
          properties:
            # drops all capabilities
            dropCapabilities:
              - ALL
            # runs container as non-root user
            runAsNonRoot: true
            # ensures that the container runs unprivileged
            privileged: false
            # runs container in read-only mode
            readOnlyRootFilesystem: false
```
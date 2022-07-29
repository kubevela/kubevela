```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: resource-app
spec:
  components:
    - name: express-server
      type: webservice
      properties:
        image: crccheck/hello-world
        ports:
          - port: 8000
      traits:
        - type: resource
          properties:
            cpu: 2
            memory: 2Gi
            requests:
              cpu: 2
              memory: 2Gi
            limits:
              cpu: 4
              memory: 4Gi
```
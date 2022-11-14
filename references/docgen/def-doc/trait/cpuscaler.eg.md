```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: website
spec:
  components:
    - name: frontend
      type: webservice
      properties:
        image: nginx
      traits:
        - type: cpuscaler
          properties:
            min: 1
            max: 10
            cpuUtil: 60
```
```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: vela-cli
  namespace: default
spec:
  components: []
  workflow:
    steps:
    - name: list-app
      type: vela-cli
      properties:
        command:
          - vela
          - ls
```
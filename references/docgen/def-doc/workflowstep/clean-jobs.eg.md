```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: clean-jobs
  namespace: default
spec:
  components: []
  workflow:
    steps:
    - name: clean-cli-jobs
      type: clean-jobs
      properties:
        labelselector:
          "my-label": my-value
```
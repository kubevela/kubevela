```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: apply-terraform-provider
  namespace: default
spec:
  components: []
  workflow:
    steps:
    - name: provider
      type: apply-terraform-provider
      properties:
        type: alibaba
        name: my-alibaba-provider
        accessKey: <accessKey>
        secretKey: <secretKey>
        region: cn-hangzhou
```
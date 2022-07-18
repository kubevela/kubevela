```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: provision-cloud-resource-eip
spec:
  components:
    - name: sample-eip
      type: alibaba-eip
      properties:
        writeConnectionSecretToRef:
          name: eip-conn
```

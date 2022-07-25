```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: oss-cloud-source
spec:
  components:
    - name: sample-oss
      type: alibaba-oss
      properties:
        bucket: vela-website
        acl: private
        writeConnectionSecretToRef:
          name: oss-conn
```

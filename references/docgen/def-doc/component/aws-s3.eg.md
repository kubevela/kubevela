```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: s3-cloud-source
spec:
  components:
    - name: sample-s3
      type: aws-s3
      properties:
        bucket: vela-website-20211019
        acl: private

        writeConnectionSecretToRef:
          name: s3-conn
```

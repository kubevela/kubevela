```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: upload-to-bucket
  namespace: default
spec:
  components: []
  workflow:
    steps:
      - name: upload
        type: upload-to-bucket
        properties:
          oss:
            accessKey:
              # User can either use plaintext as input parameters `id` and `secret` or `secretRef` to specify the credentials.
              id: <your_alibaba_accesskey_id>
              secret: <your_alibaba_accesskey_secret>
              # secretRef:
              #   name: my-secret
              #   keyId: ALICLOUD_ACCESS_KEY
              #   keySecret: ALICLOUD_SECRET_KEY
            bucket: example-doc
            endpoint: oss-cn-hangzhou.aliyuncs.com
          pvc:
            name: my-pvc
            mountPath: /generated
```
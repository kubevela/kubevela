```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: app-vpc-sample
spec:
  components:
    - name: sample-vpc
      type: alibaba-vpc
      properties:
        vpc_cidr: "172.16.0.0/12"
        writeConnectionSecretToRef:
          name: vpc-conn
```

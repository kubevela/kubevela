```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: app-sls-project-sample
spec:
  components:
    - name: sample-sls-project
      type: alibaba-sls-project
      properties:
        name: kubevela-1112
        description: "Managed by KubeVela"

        writeConnectionSecretToRef:
          name: sls-project-conn
```

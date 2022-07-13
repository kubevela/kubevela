```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: app-sls-store-sample
spec:
  components:
    - name: sample-sls-store
      type: alibaba-sls-store
      properties:
        store_name: kubevela-1111
        store_retention_period: 30
        store_shard_count: 2
        store_max_split_shard_count: 2

        writeConnectionSecretToRef:
          name: sls-store-conn
```

```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: crate-aws-s3-deploy
  namespace: team-a
spec:
  components:
    - name: aws-s3
      type: crate
      properties:
        crate: aws-s3
        registry: catalog
        version: "1.0.0"
```

Send the runtime objects of the crate to managed clusters. Definitions and hub objects stay on the hub.

```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: crate-aws-s3-deploy
  namespace: team-a
spec:
  components:
    - name: aws-s3
      type: crate
      properties:
        registry: catalog
        version: "1.1.0"
        placement:
          clusterLabelSelector:
            region: east
```


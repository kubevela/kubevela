The example reads from a crate registry named `catalog`. Add it first with
`vela crate registry add catalog <oci-url>`, or leave `registry` empty to use the
configured default registry.

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

`namespace` is not a property. If you set it, admission rejects the Application. The definitions install into the namespace of this Application, and that namespace must exist. On a managed cluster, a runtime object with no namespace also goes into this namespace, and the namespace must exist there too. KubeVela does not create it.

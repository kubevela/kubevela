Requires the `EnableCrateComponent` feature gate. Install or upgrade the chart with
`--set featureGates.enableCrateComponent=true`. With the gate disabled the
ComponentDefinition is still installed and the `vela/crate` package still compiles,
but no render service is registered, so rendering a `type: crate` component fails.

```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: comp-s3
  namespace: vela-system
spec:
  components:
    - name: s3
      type: crate
      properties:
        # crate: s3          # optional, defaults to the component name
        registry: crates      # optional, empty means the configured default registry
        version: "1.0.0"       # optional, empty means the latest published version
        # placement:           # optional, clusters for objects marked runtime
        #   clusters:
        #     - spoke
```

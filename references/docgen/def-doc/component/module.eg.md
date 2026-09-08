Requires the `EnableModuleComponent` feature gate. Install or upgrade the chart with
`--set featureGates.enableModuleComponent=true`. With the gate disabled the
ComponentDefinition is still installed and the `vela/module` package still compiles,
but no render service is registered, so rendering a `type: module` component fails.

```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: comp-s3
  namespace: vela-system
spec:
  components:
    - name: s3
      type: module
      properties:
        # module: s3          # optional, defaults to the component name
        registry: modules      # optional, empty means the configured default registry
        namespace: vela-system # optional, empty means the default system namespace
        version: "1.0.0"       # optional, empty means the latest published version
```

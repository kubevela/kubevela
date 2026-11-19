Requires the `EnableAddonComponent` feature gate. Install or upgrade the chart with
`--set featureGates.enableAddonComponent=true`. With the gate disabled the
ComponentDefinition is still installed, but rendering a `type: addon` component fails
with a message naming the gate.

```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: comp-fluxcd
  namespace: vela-system
spec:
  components:
    - name: fluxcd
      type: addon
      properties:
        # addon: fluxcd        # optional, defaults to the component name
        version: "3.0.2"       # optional, empty means the latest stable version
        registry: KubeVela     # optional, empty means the configured default registry
        skipVersionValidation: false
        properties:            # optional, the addon's own parameters
          {}
```

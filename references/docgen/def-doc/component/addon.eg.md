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
        skipVersionValidate: false
        properties:            # optional, the addon's own parameters
          {}
```

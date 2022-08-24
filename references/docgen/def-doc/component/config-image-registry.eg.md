```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: image-dev
  namespace: vela-system
  labels:
    "app.oam.dev/source-of-truth": "from-inner-system"
    "config.oam.dev/catalog": "velacore-config"
    "config.oam.dev/type": "config-image-registry"
    project: abc
spec:
  components:
    - name: image-dev
      type: config-image-registry
      properties:
        registry: "registry.cn-beijing.aliyuncs.com"
        auth:
          username: "my-username"
          password: "my-password"
          email: "a@gmail.com"
```
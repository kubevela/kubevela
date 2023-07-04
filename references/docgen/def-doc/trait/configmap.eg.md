```yaml
# sample.yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: configmap-app
spec:
  components:
    - name: express-server
      type: webservice
      properties:
        image: oamdev/hello-world
        ports:
          - port: 8000
      traits:
        - type: storage
          properties:
            configMap:
              - name: test1
                mountPath: /test/mount/cm
                # Mount ConfigMap to Env
                mountToEnv:
                  envName: TEST_ENV
                  configMapKey: key1
                data:
                  key1: value1
                  key2: value2
```

```yaml
kind: Application
apiVersion: core.oam.dev/v1beta1
metadata:
  name: test-config
  namespace: "config-e2e-test"
spec:
  components: []
  workflow:
    steps:
    - name: write-config
      type: create-config
      properties:
        name: test
        config: 
          key1: value1
          key2: 2
          key3: true
          key4: 
            key5: value5
    - name: read-config
      type: read-config
      properties:
        name: test
      outputs:
      - fromKey: config
        name: read-config
    - name: delete-config
      type: delete-config
      properties:
        name: test
```
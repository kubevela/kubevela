```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: bucket-app
spec:
  components:
    - name: bucket-comp
      type: kustomize
      # ... omitted for brevity
      traits:
        - type: kustomize-patch
          properties:
            patches:
              - patch: |-
                  apiVersion: v1
                  kind: Pod
                  metadata:
                    name: not-used
                    labels:
                      app.kubernetes.io/part-of: test-app
                target:
                  labelSelector: "app=podinfo"
```
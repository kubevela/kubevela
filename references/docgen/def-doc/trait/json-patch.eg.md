```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: busybox
spec:
  components:
    - name: busybox
      type: webservice
      properties:
        image: busybox
        cmd: ["sleep", "86400"]
        labels:
          pod-label-key: pod-label-value
          to-delete-label-key: to-delete-label-value
      traits:
        # the json patch can be used to add, replace and delete fields
        # the following part will
        # 1. add `deploy-label-key` to deployment labels
        # 2. set deployment replicas to 3
        # 3. set `pod-label-key` to `pod-label-modified-value` in pod labels
        # 4. delete `to-delete-label-key` in pod labels
        # 5. add sidecar container for pod
        - type: json-patch
          properties:
            operations:
              - op: add
                path: "/spec/replicas"
                value: 3
              - op: replace
                path: "/spec/template/metadata/labels/pod-label-key"
                value: pod-label-modified-value
              - op: remove
                path: "/spec/template/metadata/labels/to-delete-label-key"
              - op: add
                path: "/spec/template/spec/containers/1"
                value:
                  name: busybox-sidecar
                  image: busybox:1.34
                  command: ["sleep", "864000"]
```
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
        # the json merge patch can be used to add, replace and delete fields
        # the following part will
        # 1. add `deploy-label-key` to deployment labels
        # 2. set deployment replicas to 3
        # 3. set `pod-label-key` to `pod-label-modified-value` in pod labels
        # 4. delete `to-delete-label-key` in pod labels
        # 5. reset `containers` for pod
        - type: json-merge-patch
          properties:
            metadata:
              labels:
                deploy-label-key: deploy-label-added-value
            spec:
              replicas: 3
              template:
                metadata:
                  labels:
                    pod-label-key: pod-label-modified-value
                    to-delete-label-key: null
                spec:
                  containers:
                    - name: busybox-new
                      image: busybox:1.34
                      command: ["sleep", "864000"]
```
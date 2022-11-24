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
          label-key: label-value
          to-delete-label-key: to-delete-label-value
      traits:
        - type: affinity
          properties:
            podAffinity:
              preferred:
                - weight: 1
                  podAffinityTerm:
                    labelSelector:
                      matchExpressions:
                        - key: "secrity"
                          values: ["S1"]
                    namespaces: ["default"]
                    topologyKey: "kubernetes.io/hostname"
```
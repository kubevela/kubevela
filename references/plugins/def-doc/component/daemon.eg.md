```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: addon-node-exporter
  namespace: vela-system
spec:
  components:
  - name: node-exporter
    type: daemon    
    properties:
      image: prom/node-exporter
      imagePullPolicy: IfNotPresent
      volumeMounts:
        hostPath:
        - mountPath: /host/sys
          mountPropagation: HostToContainer
          name: sys
          path: /sys
          readOnly: true
        - mountPath: /host/root
          mountPropagation: HostToContainer
          name: root
          path: /
          readOnly: true
    traits:
    - properties:
        args:
        - --path.sysfs=/host/sys
        - --path.rootfs=/host/root
        - --no-collector.wifi
        - --no-collector.hwmon
        - --collector.filesystem.ignored-mount-points=^/(dev|proc|sys|var/lib/docker/.+|var/lib/kubelet/pods/.+)($|/)
        - --collector.netclass.ignored-devices=^(veth.*)$
      type: command
    - properties:
        annotations:
          prometheus.io/path: /metrics
          prometheus.io/port: "8080"
          prometheus.io/scrape: "true"
        port:
        - 9100
      type: expose
    - properties:
        cpu: 0.1
        memory: 250Mi
      type: resource
```
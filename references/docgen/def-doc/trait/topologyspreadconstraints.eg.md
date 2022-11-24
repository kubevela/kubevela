```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: application-with-topologyspreadconstraints
spec:
  components:
    - name: busybox-runner
      type: worker
      properties:
        image: busybox
        cmd:
          - sleep
          - '1000'
      traits:
      - type: topologyspreadconstraints
        properties:
          constraints: 
          - topologyKey: zone
            labelSelector:
              matchLabels: 
                zone: us-east-1a
            maxSkew: 1
            whenUnsatisfiable: DoNotSchedule
            minDomains: 1
            nodeAffinityPolicy: Ignore
            nodeTaintsPolicy: Ignore
          - topologyKey: node
            labelSelector:
              matchExpressions:
                - key: foo 
                  operator: In
                  values: 
                  - abc
            maxSkew: 1
            whenUnsatisfiable: ScheduleAnyway 
            minDomains: 1
            nodeAffinityPolicy: Ignore
            nodeTaintsPolicy: Ignore
```
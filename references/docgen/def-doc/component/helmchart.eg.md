```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: app-helmchart
spec:
  components:
    - name: my-chart
      type: helmchart
      properties:
        chart:
          source: podinfo
          repoURL: https://stefanprodan.github.io/podinfo
          version: "6.11.1"
        release:
          name: podinfo
          namespace: default
        values:
          replicaCount: 2
          resources:
            limits:
              memory: 256Mi
              cpu: 100m
        options:
          createNamespace: true
          includeCRDs: true
          skipTests: true
```

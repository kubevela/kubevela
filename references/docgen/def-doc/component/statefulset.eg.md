```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: postgres
spec:
  components:
    - name: postgres
      type: statefulset
      properties:
        cpu: "1"
        exposeType: ClusterIP
        # see https://hub.docker.com/_/postgres
        image: docker.io/library/postgres:16.4
        memory: 2Gi
        ports:
          - expose: true
            port: 5432
            protocol: TCP
        env:
        - name: POSTGRES_DB
          value: mydb
        - name: POSTGRES_USER
          value: postgres
        - name: POSTGRES_PASSWORD
          value: kvsecretpwd123
      traits:
        - type: scaler
          properties:
            replicas: 1
        - type: storage
          properties:
            pvc:
              - name: "postgresdb-pvc"
                storageClassName: local-path
                resources:
                  requests:
                    storage: "2Gi"
                mountPath: "/var/lib/postgresql/data"
```
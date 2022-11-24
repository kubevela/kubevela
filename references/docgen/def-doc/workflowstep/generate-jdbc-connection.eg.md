```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: jdbc
spec:
  components:
    - name: db
      type: alibaba-rds
      properties:
        instance_name: favorite-links
        database_name: db1
        account_name: oamtest
        password: U34rfwefwefffaked
        security_ips: [ "0.0.0.0/0" ]
        privilege: ReadWrite
        writeConnectionSecretToRef:
          name: db-conn
    - name: express-server
      type: webservice
      properties:
        image: crccheck/hello-world
        port: 8000

  workflow:
    steps:
      - name: jdbc
        type: generate-jdbc-connection
        outputs:
          - name: jdbc
            valueFrom: jdbc
        properties:
          name: db-conn
          namespace: default
      - name: apply
        type: apply-component
        inputs:
          - from: jdbc
            parameterKey: env
        properties:
          component: express-server
```
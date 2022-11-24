```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: rds-app
  namespace: project-1
spec:
  components:
    - name: db
      type: alibaba-rds
      properties:
        instance_name: db
        account_name: kubevela
        password: my-password
        writeConnectionSecretToRef:
          name: project-1-rds-conn-credential
  policies:
    - name: env-policy
      type: env-binding
      properties:
        envs:
          # 部署 RDS 给杭州集群
          - name: hangzhou
            placement:
              clusterSelector:
                name: cluster-hangzhou
            patch:
              components:
                - name: db
                  type: alibaba-rds
                  properties:
                    # region: hangzhou
                    instance_name: hangzhou_db
          # 部署 RDS 给香港集群
          - name: hongkong
            placement:
              clusterSelector:
                name: cluster-hongkong
              namespaceSelector:
                name: hk-project-1
            patch:
              components:
                - name: db
                  type: alibaba-rds
                  properties:
                    # region: hongkong
                    instance_name: hongkong_db
                    writeConnectionSecretToRef:
                      name: hk-project-rds-credential

  workflow:
    steps:
      # 部署 RDS 给杭州区用
      - name: deploy-hangzhou-rds
        type: deploy-cloud-resource
        properties:
          env: hangzhou
      # 将给杭州区用的 RDS 共享给北京区
      - name: share-hangzhou-rds-to-beijing
        type: share-cloud-resource
        properties:
          env: hangzhou
          placements:
            - cluster: cluster-beijing
      # 部署 RDS 给香港区用
      - name: deploy-hongkong-rds
        type: deploy-cloud-resource
        properties:
          env: hongkong
      # 将给香港区用的 RDS 共享给香港区其他项目用
      - name: share-hongkong-rds-to-other-namespace
        type: share-cloud-resource
        properties:
          env: hongkong
          placements:
            - cluster: cluster-hongkong
              namespace: hk-project-2
            - cluster: cluster-hongkong
              namespace: hk-project-3
```
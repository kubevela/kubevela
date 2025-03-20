```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: build-source-code
  namespace: default
spec:
  components: []
  workflow:
    steps:
      - name: build
        type: build-source-code
        properties:
          image: node:14.17
          cmd: "yarn install && yarn build"
          repo:
            url: https://github.com/open-gitops/website.git
            branch: main
          pvc:
            name: my-pvc
            storageClassName: my-storage
            mountPath: /generated
```
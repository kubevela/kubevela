```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: build-push-image
  namespace: default
spec:
  components:
  - name: my-web
    type: webservice
    properties:
      image: fogdong/simple-web-demo:v1
      ports:
        - port: 80
          expose: true
  workflow:
    steps:
    - name: create-git-secret
      type: export2secret
      properties:
        secretName: git-secret
        data:
          token: <git token>
    - name: create-image-secret
      type: export2secret
      properties:
        secretName: image-secret
        kind: docker-registry
        dockerRegistry:
          username: <docker username>
          password: <docker password>
    - name: build-push
      type: build-push-image
      properties:
        git: https://github.com/FogDong/simple-web-demo
        branch: main
        image: fogdong/simple-web-demo:v1
        credentials:
          image:
            name: image-secret
    - name: apply-comp
      type: apply-component
      properties:
        component: my-web
```
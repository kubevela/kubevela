```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: bookinfo
spec:
  components:
    - name: productpage
      type: webservice
      properties:
        image: nocalhost-docker.pkg.coding.net/nocalhost/bookinfo/productpage:latest
        port: 9080
      traits:
        - type: nocalhost
          properties:
            port: 9080
            gitUrl: https://github.com/nocalhost/bookinfo-productpage.git
            image: nocalhost-docker.pkg.coding.net/nocalhost/dev-images/python:3.7.7-slim-productpage-with-pydevd
            shell: "bash"
            workDir: "/opt/work"
            resources:
              limits:
                memory: 1Gi
                cpu: "1"
              requests:
                memory: 512Mi
                cpu: "0.5"
            debug:
              remoteDebugPort: 9009
            hotReload: true
            sync:
              type: send
              filePattern:
                - ./
              ignoreFilePattern:
                - .git
                - .idea
            command:
              run:
                - sh
                - run.sh
              debug:
                - sh
                - debug.sh
            env:
              - name: "foo"
                value: "bar"
            portForward:
              - 39080:9080
```
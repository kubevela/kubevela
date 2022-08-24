```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: first-vela-workflow
  namespace: default
spec:
  components:
  - name: express-server
    type: webservice
    properties:
      image: oamdev/hello-world
      port: 8000
    traits:
    - type: ingress
      properties:
        domain: testsvc.example.com
        http:
          /: 8000
  workflow:
    steps:
      - name: express-server
        type: depends-on-app
        properties:
          name: another-app
          namespace: default
```

`depends-on-app` will check if the cluster has the application with `name` and `namespace` given in properties.
If the application exists, it will hang the next step until the application is running.
If the application does not exist, KubeVela will check the ConfigMap with the same name, and read the config of the Application and apply to cluster.
The ConfigMap is like below: the `name` and `namespace` of the ConfigMap is the same in properties.
In data, the `key` must be specified by `application`, and the `value` is the yaml of the deployed application yaml.

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: myapp
  namespace: vela-system
data:
  application: 
    <app yaml file>
```
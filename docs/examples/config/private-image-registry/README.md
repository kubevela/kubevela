# How to config a private registry

* Step 1: Make sure there is a image-registry template

```bash
$ vela config-template list -A
NAMESPACE       NAME                    ALIAS                                   SCOPE   SENSITIVE       CREATED-TIME                 
vela-system     dex-connector           Dex Connector                           system  false           2022-10-12 23:48:05 +0800 CST
vela-system     helm-repository         Helm Repository                         project false           2022-10-14 12:04:58 +0800 CST
vela-system     image-registry          Image Registry                          project false           2022-10-13 15:39:37 +0800 CST

# View the document of the properties
$ vela config-template show image-registry
```

If not exist, please enable the VelaUX addon firstly.

* Step 2: Create a config and distribute to the developer namespace

```bash
# Create a developer environment(namespace)
$ vela env init developer --namespace developer

# Create a registry config for the docker hub, you could change the username and password
$ vela config create private-demo --template image-registry --target developer registry=index.docker.io auth.username=demo auth.password=demo
```

* Step 3: Create a application to use the private registry.

```bash
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: private-image
  namespace: developer
spec:
  components:
  - name: private-image
    properties:
      cpu: "0.5"
      image: private/nginx
      imagePullSecrets:
      - private-demo
    type: webservice
```

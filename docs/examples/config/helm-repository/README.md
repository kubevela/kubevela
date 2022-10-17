# How to config a helm repository

* Step 1: Make sure there is a helm-repository template

```bash
$ vela config-template list -A
NAMESPACE       NAME                    ALIAS                                   SCOPE   SENSITIVE       CREATED-TIME                 
vela-system     dex-connector           Dex Connector                           system  false           2022-10-12 23:48:05 +0800 CST
vela-system     helm-repository         Helm Repository                         project false           2022-10-14 12:04:58 +0800 CST
vela-system     image-registry          Image Registry                          project false           2022-10-13 15:39:37 +0800 CST

# View the document of the properties
$ vela config-template show helm-repository

```

If not exist, please enable the flux addon firstly.

* Step 2: Create a config and distribute to the developer namespace

```bash
# Create a developer environment(namespace)
$ vela env init developer --namespace developer

# Create a registry config for the chart repository hosted by KubeVela team
$ vela config create kubevela-core --template helm-repository --target developer url=https://charts.kubevela.net/core
```

* Step 3: Create a application to use the helm repository

```bash
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: helm-app
  namespace: developer
spec:
  components:
  - name: helm
    properties:
      chart: vela-rollout
      repoType: helm
      retries: 3
      secretRef: kubevela-core
      url: https://charts.kubevela.net/core
    type: helm
```

# How to store and use configurations

## General

- list all configuration types
```shell
$ vela components --label custom.definition.oam.dev/catalog.config.oam.dev=velacore-config
NAME                  	DEFINITION
config-dex-connector  	autodetects.core.oam.dev
config-helm-repository	autodetects.core.oam.dev
config-image-registry 	autodetects.core.oam.dev
terraform-azure       	autodetects.core.oam.dev
terraform-baidu       	autodetects.core.oam.dev
```

```json
# Get http://127.0.0.1:8000/api/v1/configs

[
 {
  "definitions": [
   "config-dex-connector"
  ],
  "name": "Dex Connectors",
  "type": "dex-connector"
 },
 {
  "definitions": [
   "config-helm-repository"
  ],
  "name": "Helm Repository",
  "type": "helm-repository"
 },
 {
  "definitions": [
   "config-image-registry"
  ],
  "name": "Image Registry",
  "type": "image-registry"
 },
 null,
 {
  "definitions": [
   "terraform-baidu"
  ],
  "name": "Terraform Cloud Provider",
  "type": "terraform-provider"
 }
]
```

- list all configurations

```shell
$ kubectl get secret -n vela-system -l=config.oam.dev/catalog=velacore-config
NAME                 TYPE                             DATA   AGE
image-registry-dev   kubernetes.io/dockerconfigjson   1      3h51m
```

## Image registry

- Create a config for an image registry

```shell
$ vela up -f app-config-image-registry-account-auth.yaml
Applying an application in vela K8s object format...
I0323 10:45:25.347102   85930 apply.go:107] "creating object" name="config-image-registry-account-auth-dev" resource="core.oam.dev/v1beta1, Kind=Application"
✅ App has been deployed 🚀🚀🚀
    Port forward: vela port-forward config-image-registry-account-auth-dev
             SSH: vela exec config-image-registry-account-auth-dev
         Logging: vela logs config-image-registry-account-auth-dev
      App status: vela status config-image-registry-account-auth-dev
        Endpoint: vela status config-image-registry-account-auth-dev
 --endpoint%
 
 
$ kubectl get secret -n vela-system -l=config.oam.dev/catalog=velacore-config
NAME                 TYPE                             DATA   AGE
image-registry-dev   kubernetes.io/dockerconfigjson   1      77s
```

- Deliver the config secret to working cluster

```shell
$ vela cluster list
CLUSTER	TYPE           	ENDPOINT                  	ACCEPTED	LABELS
local  	Internal       	-                         	true
bj     	X509Certificate	https://123.57.73.107:6443	true

$ vela up -f app-deliever-secret.yaml
```

- Deploy an application who needs to pull images from the private image registry

```shell
$ export KUBECONFIG=~/.kube/config-bj
$ kubectl get secret -n vela-system -l=config.oam.dev/catalog=velacore-config
NAME                 TYPE                             DATA   AGE
image-registry-dev   kubernetes.io/dockerconfigjson   1      120s

$ vela up -f app-validate-imagePullSecret.yaml
```
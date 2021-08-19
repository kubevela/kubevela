# WorkFlow with OCM

In this tutorial, you will create an ack cluster as a production environment and deploy the configured app
to this production environment.

## Prerequisites

- In order to follow the guide, you will need a Kubernetes cluster version 1.20+ as control-plane cluster, and 
the cluster's APIServer has an external IP.

- Store the AK/AS of Alibaba Cloud to the Secret. 

    ```shell
    export ALICLOUD_ACCESS_KEY=xxx; export ALICLOUD_SECRET_KEY=yyy
    ```
    
    ```shell
    # If you'd like to use Alicloud Security Token Service, also export `ALICLOUD_SECURITY_TOKEN`.
    export ALICLOUD_SECURITY_TOKEN=zzz
    ```
    
    ```shell
    sh hack/prepare-alibaba-credentials.sh
    ```
    
    ```shell
    $ kubectl get secret -n vela-system
    NAME                                         TYPE                                  DATA   AGE
    alibaba-account-creds                        Opaque                                1      11s
    ```

- Install Definitions
   ```shell
   kubectl apply -f definitions
   ```

## Create Initializer terraform-alibaba

Initializer terraform-alibaba will create an environment which allows users use terraform to create cloud resource on aliyun.

```shell
kubectl apply -f initializers/init-terraform-alibaba.yaml
```

It will take few minutes to wait the `PHASE` of Initializer `terraform-alibaba` to be `success`.

```shell
$ kubectl get initializers.core.oam.dev -n vela-system
NAMESPACE     NAME                  PHASE     AGE
vela-system   terraform-alibaba     success   94s
```

## Create Initializer managed-cluster

Initializer managed-cluster can create an ack cluster and use OCM to manage the cluster.

1. You should set the `hubAPIServer` to the public network address in `init-managed-cluster.yaml`
```yaml
# init-managed-cluster.yaml
- name: register-ack
   type: register-cluster
   inputs:
    ...
   properties:
     # user should set public network address of your control-plane cluster APIServer
     hubAPIServer: {{ public network address of APIServer }}
```

2. Apply the Initializer managed-cluster
```shell
kubectl apply -f initializers/init-managed-cluster.yaml
```

It will take 15 to 20 minutes to create an ack cluster, please wait until the status of `managed-cluster` to be `success`. 

```shell
$ kubectl get initializers.core.oam.dev -n vela-system
NAMESPACE     NAME                  PHASE     AGE
vela-system   managed-cluster       success   45m
```

3. Check the new ack cluster has been registered

```shell
$ kubectl get managedclusters.cluster.open-cluster-management.io
NAME     HUB ACCEPTED   MANAGED CLUSTER URLS         JOINED   AVAILABLE   AGE
poc-01   true          {{ APIServer address }}       True     True        30s
```


## Deploy the resource to ack cluster

```shell
kubectl apply -f app.yaml
```

check the app `workflow-demo` was created successfully

```shell
$ kubectl get app workflow-demo
NAME            COMPONENT         TYPE         PHASE     HEALTHY   STATUS   AGE
workflow-demo   podinfo-server    webservice   running   true               7s
```

use kubectl connect to the managed-cluster `poc-01` and check the resources in the app 
are successfully deployed to the cluster `poc-01`.

```shell
$ kubectl get deployments
NAME             READY   UP-TO-DATE   AVAILABLE   AGE
podinfo-server   1/1     1            1           40s
```
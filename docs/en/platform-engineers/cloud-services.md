# Cloud service

In this tutorial, we will add a Alibaba Cloud's RDS service as a new workload type in KubeVela.

## Step 1: Install and configure Crossplane (v0.13)

In this tutorial, we use Crossplane as the cloud resource operator for Kubernetes. 

<details>

To make this process more easier, we provide all the needed scripts in [this folder](https://github.com/oam-dev/kubevela/tree/master/docs/examples/kubecondemo). 

Please do:
```console
$ cd examples/kubecondemo/
```
You also need to have the Access Key and Secret to your Alibaba Cloud account by hand and then follow the steps below.

* Create crossplane namespace: `kubectl create ns crossplane-system`
* Install crossplane helm chart: `helm install crossplane  charts/crossplane/ --namespace crossplane-system`
* Install crossplane cli: `curl -sL https://raw.githubusercontent.com/crossplane/crossplane/release-0.13/install.sh | sh`
* Add crossplane to `PATH`:  `sudo mv kubectl-crossplane /usr/local/bin`
* Configure cloud provider(Alibaba Cloud) 
  * Add cloud provider: `kubectl crossplane install provider crossplane/provider-alibaba:v0.3.0`
  * Create provider secret: `kubectl create secret generic alibaba-creds --from-literal=accessKeyId=<change here> --from-literal=accessKeySecret=<change here> -n crossplane-system`
  * Configure the provider: `kubectl apply -f script/provider.yaml`
* Configure infrastructure: `kubectl crossplane install configuration crossplane/getting-started-with-alibaba:v0.13`

So far we have configured Crossplane on the cluster.

</details>

## Step 2: Add Workload Definition

First, register the `rds` workload type to KubeVela: 

```console
kubectl apply -f script/def_db.yaml
``` 

Check the new workload type is added:   
```console
$ vela workloads
```   

## Step 3: Verify RDS workload type in Appfile

Now in the Appfile, we claim an RDS instance with workload type of `rds`:

``` yaml
name: lab3

services:
  ...
  database:
    type: rds
    name: alibabaRds
    ...
```

> Please check the full application sample in [tutorial folder](https://github.com/oam-dev/kubevela/blob/master/docs/examples/kubecondemo/vela.yaml).

Next, we could deploy the application with `$ vela up`

**(Optional) Verify the database status**

<details>

We can verify the status of database (usually takes 6 min to be ready):

```console
$ kubectl get postgresqlinstance`
```

When the database is ready, you can see the `READY: True` output.

</details>

### Access the application

In the Appfile we added a route trait with domain of `kubevela.kubecon.demo`.

So the application should be accessable via:

```
$ curl -H "Host:kubevela.kubecon.demo" http://localhost:8080
```


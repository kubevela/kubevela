# Cloud service

In this demo, we will add an RDS database from Alibaba Cloud to our application.

## What is cloud service

Cloud service refers to the managed cloud resources. For example, you can buy a PostgreSql database from a cloud vendor instead of setting up your own. Cloud service are normally outside of your Kubernetes cluster, but logically they are still part of your application. KubeVela provides application centric view of your applications. It will treat every service the same.

## Crossplane

Crossplane is an open source Kubernetes add-on that extends any cluster with the ability to provision and manage cloud infrastructure, services, and applications using kubectl, GitOps, or any tool that works with the Kubernetes API. The benefit of using Crossplane is it will provide centralized control plane disregard where your cluster is.

KubeVela will delegate the lifecycle management of cloud service to Crossplane.

## Install Crossplane (This demo uses crossplane version 0.13)

You will need a Kubernetes cluster ver> 1.16 (Minikube and Kind clusters are fine).
Also you will need to have KubeVela installed on the cluster.
To provision a cloud resource, you need to have the Access Key and Secret to your Alibaba cloud account.

* Create crossplane namespace: `kubectl create ns crossplane-system`
* Install crossplane helm chart: `helm install crossplane  charts/crossplane/ --namespace crossplane-system`
* Install crossplane cli: `curl -sL https://raw.githubusercontent.com/crossplane/crossplane/release-0.13/install.sh | sh`
* Add crossplane to `PATH`:  `sudo mv kubectl-crossplane /usr/local/bin`
* Configure cloud provider(Alibaba Cloud) 
  * Add cloud provider: `kubectl crossplane install provider crossplane/provider-alibaba:v0.3.0`
  * Create provider secret: `kubectl create secret generic alibaba-creds --from-literal=accessKeyId=<change here> --from-literal=accessKeySecret=<change here> -n crossplane-system`
  * Configure the provider: `kubectl apply -f ../../examples/kubecondemo/script/provider.yaml`
* Configure infrastructure: `kubectl crossplane install configuration crossplane/getting-started-with-alibaba:v0.13`

So far we have configured Crossplane on the cluster.

## Import the database workload definition

First, register the db workload definition:   
`kubectl apply -f ../../examples/kubecondemo/script/def_db.yaml`   
The webservice workload is different from the default version so we have to overwrite it.   
`kubectl apply -f ../../examples/kubecondemo/script/webservice.yaml`   
Don't forget to update vela:   
`vela system update`   

## Apply the appfile

In the Appfile, we claim an RDS instance just like other services:

``` yaml
database:
    type: rds
    name: alibabaRds
    ...
```

Next, we start the application:   
`vela up -f ../../examples/kubecondemo/vela.yaml`

## Verify the database status

Under the hood, we can verify the status of database(usually takes >6 min to be ready):   
`kubectl get postgresqlinstance`

When the database is ready, you can see the `READY: True` output.

## Access the web-ui

In the Appfile we added a route trait. To access the web-ui, please check out the [route](set-route.md) documentation.


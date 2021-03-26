# Kubecon 2020 NA Kubevela Tutorial

> :warning: This is an outdated tutorial only applies to the old version of kubevela.
Before you read, you need to know what you are doing.

## Pre-requisites

* Kubernetes cluster version >1.16
(minikube or kind are fine)
* Verify with `kubectl config current-context` and `kubectl version`
* One of the crossplane supported public cloud (AWS, Azure, Alibaba Cloud, GCK) access key and secret
* Install Crossplane(later)
* Download KubeVela release from [release page](https://github.com/oam-dev/kubevela/releases/tag/v0.0.9)
* Unpack the package and add it to `PATH` by running `sudo mv ./vela /usr/local/bin/vela`
* Run `vela install`

## Lab 1: Use vela to deploy a simple application

### Purpose: Showcase the simple to use, application centric vela user interfaces.

* Sync with cluster `vela system update`
* List installed workloads `vela workloads`
* List installed traits `vela traits`
* Deploy a simple application with 

  ```
  vela svc deploy back -t worker --image crccheck/hello-world --app lab1
  vela svc deploy front -t webservice --image crccheck/hello-world --port 8000 --app lab1
  ```

* Show application status `vela app show lab1`

## Lab 2: Add and apply KubeWatch
  
### Purpose: Showcase the steps to add and use capacity from community

* Create a [slack bot](https://api.slack.com/apps?new_app=1)
* Add a cap center `vela cap center config mycap https://github.com/oam-dev/catalog/tree/master/registry`
* Check capabilities `vela cap ls`
* Install the kubewatch capability `vela cap add mycap/kubewatch`
* Create an application `vela comp deploy mycomp -t webservice --image crccheck/hello-world --port 8000 --app lab2`
* Add kubewatch trait to the application `vela kubewatch mycomp --app lab2 --webhook https://hooks.slack.com/<yourid>`
* Check the slack channel to verify the notifications

## Lab 3: Manage cloud resource and applications in application centric way

### Purpose: Showcase the application centric view of appfile

### Install Crossplane (This lab uses crossplane version 0.13)

Also the examples are based on Alibaba Cloud settings

* Create crossplane namespace: `kubectl create ns crossplane-system`
* Install crossplane helm chart: `helm install crossplane  charts/crossplane/ --namespace crossplane-system`
* Install crossplane cli: `curl -sL https://raw.githubusercontent.com/crossplane/crossplane/release-0.13/install.sh | sh`
* Add crossplane to `PATH`:  `sudo mv kubectl-crossplane /usr/local/bin`
* Configure cloud provider(Alibaba Cloud) 
  * Add cloud provider: `kubectl crossplane install provider crossplane/provider-alibaba:v0.3.0`
  * Create provider secret: `kubectl create secret generic alibaba-creds --from-literal=accessKeyId=<change here> --from-literal=accessKeySecret=<change here> -n crossplane-system`
  * Configure the provider: `kubectl apply -f script/provider.yaml`
* Configure infrastructure: `kubectl crossplane install configuration crossplane/getting-started-with-alibaba:v0.13`

### Import the database workload definition

First, register the db workload definition:
`kubectl apply -f script/def_db.yaml`
The webservice workload is customized a little.
`kubectl apply -f script/webservice.yaml`
Don't forget to update vela:
`vela system update`

### Apply the appfile

`vela up`

### Access the web-ui

If you have a cluster supporting Ingress, the route trait will work.
`kubectl get ingress` command will show the ip address of the web-ui. Copy that service and add the `<ip address> kubevela.kubecon.demo ` record to your local machine's `/etc/hosts`. Then you may access the GUI from web browser.

If you don't have Ingress installed, the eaisest way to access the demo app is through port forwarding :`kubectl port-forward <your webui pod name> 8080` and access it from browser using `http://localhost:8080`.

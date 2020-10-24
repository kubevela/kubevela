# Kubecon 2020 NA Kubevela Tutorial

## Pre-requisites

* Kubernetes cluster version >1.16
(minikube or kind are fine)
* Verify with `kubectl config current-context` and `kubectl version`
* One of the crossplane supported public cloud (AWS, Azure, Alibaba Cloud, GCK) access key and secret
* Install Crossplane(later)
* Download KubeVela release from [release page](https://github.com/oam-dev/kubevela/releases)
* Unpack the package and add it to `PATH` by running `sudo mv ./vela /usr/local/bin/vela`
* Run `vela install`

## Lab 1: Use vela to deploy a simple application

### Purpose: Showcase the simple to use, application centric vela user interfaces.

* Sync with cluster `vela system update`
* List installed workloads `vela workloads`
* List installed traits `vela traits`
* Deploy a simple application with 

  ```
  vela comp deploy mycomp -t backend --image crccheck/hello-world --app lab1
  vela comp deploy mycomp -t webservice --image crccheck/hello-world --port 8000 --app lab1
  ```

* Show application status `vela app show myapp`

## Lab 2: Add and apply KubeWatch
  
### Purpose: Showcase the steps to add and use capacity from community

* Create a [slack bot](https://api.slack.com/apps?new_app=1)
* Add a cap center `vela cap center config mycap https://github.com/oam-dev/catalog/tree/master/registry`
* Check capabilities `vela cap ls`
* Install the kubewatch capability `vela cap add mycap/kubewatch`
* Create an application `vela comp deploy mycomp -t webservice --image crccheck/hello-world --port 8000 --app lab2`
* Add kubewatch trait to the application `vela kubewatch mycomp --app myapp --webhook https://hooks.slack.com/<yourid>`
* Check the slack channel to verify the notifications

## Lab 3: Manage cloud resource and applications in application centric way

### Purpose: Showcase the application centric view of appfile

### Install crossplane

Follow the instruction on crossplane [documents](https://crossplane.io/docs/v0.13/)
**Don't forget to create secret**

### Apply the appfile

`vela up script/vela.yml`


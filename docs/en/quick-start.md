# Quick Start

Welcome to KubeVela! In this guide, we'll walk you through how to install KubeVela, and deploy your first simple application.

## Step 1: Install

#### 1. Setup Kubernetes cluster

- Kubernetes cluster >= v1.15.0
- kubectl installed and configured

You may pick either Minikube or KinD as local cluster testing option.

> NOTE: If you are not using minikube or kind, please make sure to [install or enable ingress-nginx](https://kubernetes.github.io/ingress-nginx/deploy/) by yourself.

##### Minikube

<details>
Follow the minikube [installation guide](https://minikube.sigs.k8s.io/docs/start/).

Once minikube is installed, create a cluster:

```bash
$ minikube start
```

Install ingress:

```bash
$ minikube addons enable ingress
```
</details>

##### KinD

<details>
Follow [this guide](https://kind.sigs.k8s.io/docs/user/quick-start/#installation) to install kind.

Then spins up a kind cluster:

```bash
cat <<EOF | kind create cluster --config=-
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
- role: control-plane
  kubeadmConfigPatches:
  - |
    kind: InitConfiguration
    nodeRegistration:
      kubeletExtraArgs:
        node-labels: "ingress-ready=true"
  extraPortMappings:
  - containerPort: 80
    hostPort: 80
    protocol: TCP
  - containerPort: 443
    hostPort: 443
    protocol: TCP
EOF
```

Then install [ingress for kind](https://kind.sigs.k8s.io/docs/user/ingress/#ingress-nginx):
```bash
$ kubectl apply -f https://raw.githubusercontent.com/kubernetes/ingress-nginx/master/deploy/static/provider/kind/deploy.yaml
```
</details>

#### 2. Get KubeVela

1. Download the latest `vela` binary from the [releases page](https://github.com/oam-dev/kubevela/releases).
2. Unpack the `vela` binary and add it to `$PATH` to get started.

```bash
$ sudo mv ./vela /usr/local/bin/vela
```

#### 3. Initialize KubeVela

Run:

```bash
$ vela install
```

This will install KubeVela server component and its dependency components.

**Verify Installation Manually (Advanced)**

<details>
Check Vela Helm Chart has been installed:
```
$ helm list -n vela-system
NAME      NAMESPACE   REVISION  ...
kubevela  vela-system 1         ...
```

Later on, check that the dependency components has been installed (they will need 5-10 minutes to complete):
```
$ helm list --all-namespaces
NAME                  NAMESPACE   REVISION  UPDATED                               STATUS    CHART                       APP VERSION
flagger               vela-system 1         2020-11-10 18:47:14.0829416 +0000 UTC deployed  flagger-1.1.0               1.1.0
keda                  keda        1         2020-11-10 18:45:15.6981827 +0000 UTC deployed  keda-2.0.0-rc3              2.0.0-rc2
kube-prometheus-stack monitoring  1         2020-11-10 18:45:37.9608079 +0000 UTC deployed  kube-prometheus-stack-9.4.4 0.38.1
kubevela              vela-system 1         2020-11-10 10:44:20.663582 -0800 PST  deployed
```

> We will introduce a `vela system health` command to check the dependencies in the future.
</details>

**Customize Your Installation (Advanced)**

<details>
We have installed the following dependency components along with Vela server component:

- [Prometheus Stack](https://github.com/prometheus-community/helm-charts/tree/main/charts/kube-prometheus-stack)
- [Cert-manager](https://cert-manager.io/)
- [Flagger](https://flagger.app/)

The config has been saved in a ConfigMap in "vela-system/vela-config":

```
$ kubectl -n vela-system get cm vela-config -o yaml
apiVersion: v1
data:
  certificates.cert-manager.io: |
    {
      "repo": "jetstack",
      "urL": "https://charts.jetstack.io",
      "name": "cert-manager",
      "namespace": "cert-manager",
      "version": "1.0.3"
    }
  flagger.app: |
  ...
kind: ConfigMap
```

User can specify their own dependencies by editing the `vela-config` ConfigMap.
Currently adding new chart or updating existing chart requires redeploying Vela:
```
$ kubectl -n vela-system edit cm vela-config
...

$ helm uninstall -n vela-system kubevela
$ helm install -n vela-system kubevela
```
</details>

## Step 2: Deploy Your First Application

**vela init**

```bash
$ vela init --render-only
Welcome to use KubeVela CLI! Please describe your application.

Environment: default, namespace: default

? What is the domain of your application service (optional):  example.com
? What is your email (optional, used to generate certification):
? What would you like to name your application (required):  testapp
? Choose the workload type for your application (required, e.g., webservice):  webservice
? What would you like to name this webservice (required):  testsvc
? specify app image crccheck/hello-world
? specify port for container 8000

Deployment config is rendered and written to vela.yaml
```

In the current directory, you will find a generated `vela.yaml` file (i.e., an Appfile):

```yaml
createTime: ...
updateTime: ...

name: testapp
services:
  testsvc:
    type: webservice
    image: crccheck/hello-world
    port: 8000
    route:
      domain: testsvc.example.com
```

**vela up**

```bash
$ vela up
Parsing vela.yaml ...
Loading templates ...

Rendering configs for service (testsvc)...
Writing deploy config to (.vela/deploy.yaml)

Applying deploy configs ...
Checking if app has been deployed...
App has not been deployed, creating a new deployment...
✅ App has been deployed 🚀🚀🚀
    Port forward: vela port-forward testapp
             SSH: vela exec testapp
         Logging: vela logs testapp
      App status: vela status testapp
  Service status: vela status testapp --svc testsvc
```

Check the status until we see route trait ready:
```bash
$ vela status testapp
About:

  Name:       testapp
  Namespace:  default
  Created at: ...
  Updated at: ...

Services:

  - Name: testsvc
    Type: webservice
    HEALTHY Ready: 1/1
    Last Deployment:
      Created at: ...
      Updated at: ...
    Routes:
      - route:  Visiting URL: http://testsvc.example.com  IP: localhost
```

**In [kind cluster setup](./install.md#kind)**, you can visit the service via localhost. In other setups, replace localhost with ingress address accordingly.

```
$ curl -H "Host:testsvc.example.com" http://localhost/
<xmp>
Hello World


                                       ##         .
                                 ## ## ##        ==
                              ## ## ## ## ##    ===
                           /""""""""""""""""\___/ ===
                      ~~~ {~~ ~~~~ ~~~ ~~~~ ~~ ~ /  ===- ~~~
                           \______ o          _,/
                            \      \       _,'
                             `'--.._\..--''
</xmp>
```
**Voila!** You are all set to go.

## Step 3: (Optional) Clean Up

<details>
Run:

```bash
$ helm uninstall -n vela-system kubevela
$ rm -r ~/.vela
```

This will uninstall KubeVela server component and its dependency components.
This also cleans up local CLI cache.

Then clean up CRDs (CRDs are not removed via helm by default):

```
$ kubectl delete crd \
  applicationconfigurations.core.oam.dev \
  applicationdeployments.core.oam.dev \
  autoscalers.standard.oam.dev \
  certificaterequests.cert-manager.io \
  certificates.cert-manager.io \
  challenges.acme.cert-manager.io \
  clusterissuers.cert-manager.io \
  components.core.oam.dev \
  containerizedworkloads.core.oam.dev \
  healthscopes.core.oam.dev \
  issuers.cert-manager.io \
  manualscalertraits.core.oam.dev \
  metricstraits.standard.oam.dev \
  orders.acme.cert-manager.io \
  podspecworkloads.standard.oam.dev \
  routes.standard.oam.dev \
  scopedefinitions.core.oam.dev \
  servicemonitors.monitoring.coreos.com \
  traitdefinitions.core.oam.dev \
  workloaddefinitions.core.oam.dev
```
</details>

## What's Next

Congratulations! You have just deployed an app using KubeVela. Here are some recommended next steps:

- Learn about the project's [motivation](./introduction.md) and [architecture](./design.md)
- Try out more [tutorials](./developers/config-enviroments.md)
- Join our community [Slack](https://cloud-native.slack.com/archives/C01BLQ3HTJA) and/or [Gitter](https://gitter.im/oam-dev/community)

Welcome onboard and sail Vela!

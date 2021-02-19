# Install KubeVela

## 1. Setup Kubernetes cluster

Requirements:
- Kubernetes cluster >= v1.15.0
- kubectl installed and configured

If you don't have K8s cluster from Cloud Provider, you may pick either Minikube or KinD as local cluster testing option.

> NOTE: If you are not using minikube or kind, please make sure to [install or enable ingress-nginx](https://kubernetes.github.io/ingress-nginx/deploy/) by yourself.

<!-- tabs:start -->

#### **Minikube**

Follow the minikube [installation guide](https://minikube.sigs.k8s.io/docs/start/).

Once minikube is installed, create a cluster:

```bash
$ minikube start
```

Install ingress:

```bash
$ minikube addons enable ingress
``` 

#### **KinD**

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

<!-- tabs:end -->

## 2. Install KubeVela Controller

These steps will install KubeVela controller and its dependency.

1. Add helm chart repo for KubeVela
    ```
    helm repo add kubevela https://kubevelacharts.oss-cn-hangzhou.aliyuncs.com/core
    ```

2. Update the chart repo
    ```
    helm repo update
    ```
   
3. Create Namespace for KubeVela controller
    ```shell script
    kubectl create namespace vela-system 
    ```

4. Install KubeVela
    ```shell script
    helm install -n vela-system kubevela kubevela/vela-core
    ```
    By default, it will enable webhook. KubeVela relies on [cert-manager](https://cert-manager.io/docs/)
    to create certificates for webhook.
    If cert-manager hasn't been installed, please refer to [cert-manager installation doc](https://cert-manager.io/docs/installation/kubernetes/).
    
    You can add an argument `--set useWebhook=false` after the command to disable the webhook if you don't want to rely on cert-manager.
    If you just want to have a try this can also work:
    ```shell script
    helm install -n vela-system kubevela kubevela/vela-core --set useWebhook=false
    ```
   
    You can also install cert-manager via kubevela chart by adding the argument `--set installCertManager=true`.
    ```shell script
    helm install -n vela-system kubevela kubevela/vela-core --set installCertManager=true
    ```
   
    If you want to try the latest master branch, add flag `--devel` in command `helm search` to choose a pre-release
    version in format `<next_version>-rc-master` which means the next release candidate version build on `master` branch,
    like `0.4.0-rc-master`.
   
    ```shell script
    $ helm search repo kubevela/vela-core -l --devel
    NAME                     	CHART VERSION        	APP VERSION          	DESCRIPTION
    kubevela/vela-core       	0.4.0-rc-master         0.4.0-rc-master         A Helm chart for KubeVela core
    kubevela/vela-core       	0.3.2  	                0.3.2                   A Helm chart for KubeVela core
    kubevela/vela-core       	0.3.1        	        0.3.1               	A Helm chart for KubeVela core
    ```
   
    And try the following command to install it.
   
    ```shell script
    helm install -n vela-system kubevela kubevela/vela-core --version <next_version>-rc-master --set useWebhook=false
    ```

## 3. Get KubeVela CLI

Here are three ways to get KubeVela Cli:

<!-- tabs:start -->

#### **Script**

**macOS/Linux**

```console
$ curl -fsSl https://kubevela.io/install.sh | bash
```

**Windows**

```console
$ powershell -Command "iwr -useb https://kubevela.io/install.ps1 | iex"
```
#### **Homebrew**
**macOS/Linux**
```console
$ brew install kubevela
```

#### **Download directly from releases**

- Download the latest `vela` binary from the [releases page](https://github.com/oam-dev/kubevela/releases).
- Unpack the `vela` binary and add it to `$PATH` to get started.

```bash
$ sudo mv ./vela /usr/local/bin/vela
```

> Known Issue(https://github.com/oam-dev/kubevela/issues/625): 
> If you're using mac, it will report that “vela” cannot be opened because the developer cannot be verified.
>
> The new version of MacOS is stricter about running software you've downloaded that isn't signed with an Apple developer key. And we haven't supported that for KubeVela yet.  
> You can open your 'System Preference' -> 'Security & Privacy' -> General, click the 'Allow Anyway' to temporarily fix it.

<!-- tabs:end -->

## 4. Sync Capability from Cluster

```bash
$ vela workloads
Automatically discover capabilities successfully ✅ Add(5) Update(0) Delete(0)

TYPE       	CATEGORY	DESCRIPTION                                                                     
+task      	workload	Describes jobs that run code or a script to completion.                         
+webservice	workload	Describes long-running, scalable, containerized services that have a stable     
           	       	network endpoint to receive external network traffic from customers. If workload
           	       	type is skipped for any service defined in Appfile, it will be defaulted to     
           	       	`webservice` type.                                                              
+worker    	workload	Describes long-running, scalable, containerized services that running at        
           	       	backend. They do NOT have network endpoint to receive external network          
           	       	traffic.                                                                        
+ingress   	trait   	Configures K8s ingress and service to enable web traffic for your service.      
           	       	Please use route trait in cap center for advanced usage.                        
+scaler    	trait   	Configures replicas for your service.                                           

NAME      	DESCRIPTION                                                                                                             
task      	Describes jobs that run code or a script to completion.                                                                 
webservice	Describes long-running, scalable, containerized services that have a stable network endpoint to receive external network
          	traffic from customers. If workload type is skipped for any service defined in Appfile, it will be defaulted to         
          	`webservice` type.                                                                                                      
worker    	Describes long-running, scalable, containerized services that running at backend. They do NOT have network endpoint to  
          	receive external network traffic.   
```

## 5. (Optional) Clean Up

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
  components.core.oam.dev \
  containerizedworkloads.core.oam.dev \
  healthscopes.core.oam.dev \
  issuers.cert-manager.io \
  manualscalertraits.core.oam.dev \
  metricstraits.standard.oam.dev \
  podspecworkloads.standard.oam.dev \
  routes.standard.oam.dev \
  scopedefinitions.core.oam.dev \
  traitdefinitions.core.oam.dev \
  workloaddefinitions.core.oam.dev
```

## 5. What's Next

Here are some recommended next steps:

- Learn KubeVela starting from its [core concepts](/en/concepts.md)
- Join `#kubevela` channel in CNCF [Slack](https://cloud-native.slack.com) and/or [Gitter](https://gitter.im/oam-dev/community)

Welcome onboard and sail Vela!

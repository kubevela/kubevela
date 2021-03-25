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

```shell script
minikube start
```

Install ingress:

```shell script
minikube addons enable ingress
``` 

#### **KinD**

Follow [this guide](https://kind.sigs.k8s.io/docs/user/quick-start/#installation) to install kind.

Then spins up a kind cluster:

```shell script
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
```shell script
kubectl apply -f https://raw.githubusercontent.com/kubernetes/ingress-nginx/master/deploy/static/provider/kind/deploy.yaml
```

<!-- tabs:end -->

## 2. Install KubeVela Controller

These steps will install KubeVela controller and its dependency.

1. Add helm chart repo for KubeVela
    ```shell script
    helm repo add kubevela https://kubevelacharts.oss-cn-hangzhou.aliyuncs.com/core
    ```

2. Update the chart repo
    ```shell script
    helm repo update
    ```
   
3. Install KubeVela
    ```shell script
    helm install --create-namespace -n vela-system kubevela kubevela/vela-core
    ```
    By default, it will enable the webhook with a self-signed certificate provided by [kube-webhook-certgen](https://github.com/jet/kube-webhook-certgen)
   
    If you want to try the latest master branch, add flag `--devel` in command `helm search` to choose a pre-release
    version in format `<next_version>-rc-master` which means the next release candidate version build on `master` branch,
    like `0.4.0-rc-master`.
   
    ```shell script
    helm search repo kubevela/vela-core -l --devel
    ```
    ```console
        NAME                     	CHART VERSION        	APP VERSION          	DESCRIPTION
        kubevela/vela-core       	0.4.0-rc-master         0.4.0-rc-master         A Helm chart for KubeVela core
        kubevela/vela-core       	0.3.2  	                0.3.2                   A Helm chart for KubeVela core
        kubevela/vela-core       	0.3.1        	        0.3.1               	A Helm chart for KubeVela core
    ```
   
    And try the following command to install it.
   
    ```shell script
    helm install --create-namespace -n vela-system kubevela kubevela/vela-core --version <next_version>-rc-master
    ```
    ```console
   NAME: kubevela
   LAST DEPLOYED: Sat Mar  6 21:03:11 2021
   NAMESPACE: vela-system
   STATUS: deployed
   REVISION: 1
   NOTES:
   1. Get the application URL by running these commands:
     export POD_NAME=$(kubectl get pods --namespace vela-system -l "app.kubernetes.io/name=vela-core,app.kubernetes.io/instance=kubevela" -o jsonpath="{.items[0].metadata.name}")
     echo "Visit http://127.0.0.1:8080 to use your application"
     kubectl --namespace vela-system port-forward $POD_NAME 8080:80
   ```

4. Install Kubevela with cert-manager (optional)
   
   If cert-manager has been installed, it can be used to take care about generating certs. 

   You need to install cert-manager before the kubevela chart.
    ```shell script
    helm repo add jetstack https://charts.jetstack.io
    helm repo update
    helm install cert-manager jetstack/cert-manager --namespace cert-manager --version v1.2.0 --create-namespace --set installCRDs=true
    ```
   
    Install kubevela with enabled certmanager:
    ```shell script
    helm install --create-namespace -n vela-system --set admissionWebhooks.certManager.enabled=true kubevela kubevela/vela-core
    ```

## 3. (Optional) Get KubeVela CLI

Here are three ways to get KubeVela Cli:

<!-- tabs:start -->

#### **Script**

**macOS/Linux**

```shell script
curl -fsSl https://kubevela.io/install.sh | bash
```

**Windows**

```shell script
powershell -Command "iwr -useb https://kubevela.io/install.ps1 | iex"
```
#### **Homebrew**
**macOS/Linux**
```shell script
brew install kubevela
```

#### **Download directly from releases**

- Download the latest `vela` binary from the [releases page](https://github.com/oam-dev/kubevela/releases).
- Unpack the `vela` binary and add it to `$PATH` to get started.

```shell script
sudo mv ./vela /usr/local/bin/vela
```

> Known Issue(https://github.com/oam-dev/kubevela/issues/625): 
> If you're using mac, it will report that “vela” cannot be opened because the developer cannot be verified.
>
> The new version of MacOS is stricter about running software you've downloaded that isn't signed with an Apple developer key. And we haven't supported that for KubeVela yet.  
> You can open your 'System Preference' -> 'Security & Privacy' -> General, click the 'Allow Anyway' to temporarily fix it.

<!-- tabs:end -->

## 4. (Optional) Sync Capability from Cluster

If you want to run application from `vela` cli, then you should sync capabilities first like below:

```shell script
vela components
```
```console
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

<details>
Run:

```shell script
helm uninstall -n vela-system kubevela
rm -r ~/.vela
```

This will uninstall KubeVela server component and its dependency components.
This also cleans up local CLI cache.

Then clean up CRDs (CRDs are not removed via helm by default):

```shell script
 kubectl delete crd \
  applicationconfigurations.core.oam.dev \
  applicationdeployments.core.oam.dev \
  components.core.oam.dev \
  containerizedworkloads.core.oam.dev \
  healthscopes.core.oam.dev \
  manualscalertraits.core.oam.dev \
  podspecworkloads.standard.oam.dev \
  scopedefinitions.core.oam.dev \
  traitdefinitions.core.oam.dev \
  workloaddefinitions.core.oam.dev
```
</details>

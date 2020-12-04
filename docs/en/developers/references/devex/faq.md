# FAQ

- [Install](#install)
  * [Error: unable to create new content in namespace cert-manager because it is being terminated](#error-unable-to-create-new-content-in-namespace-cert-manager-because-it-is-being-terminated)
  * [Error: ScopeDefinition exists](#error-scopedefinition-exists)
  * [Warning: capability rollout was not ready](#warning-capability-rollout-was-not-ready)
  * [You have reached your pull rate limit](#You-have-reached-your-pull-rate-limit)
  * [Warning: Namespace cert-manager exists](#warning-namespace-cert-manager-exists)
  
  
  
## Install

### Error: unable to create new content in namespace cert-manager because it is being terminated

Occasionally you might hit the issue as below. It happens when the last KubeVela release deletion hasn't completed.

```
$ vela install
- Installing Vela Core Chart:
install chart vela-core, version 0.1.0, desc : A Helm chart for Kube Vela core, contains 35 file
Failed to install the chart with error: serviceaccounts "cert-manager-cainjector" is forbidden: unable to create new content in namespace cert-manager because it is being terminated
failed to create resource
helm.sh/helm/v3/pkg/kube.(*Client).Update.func1
	/home/runner/go/pkg/mod/helm.sh/helm/v3@v3.2.4/pkg/kube/client.go:190
...
Error: failed to create resource: serviceaccounts "cert-manager-cainjector" is forbidden: unable to create new content in namespace cert-manager because it is being terminated
```

Take a break and try again in a few seconds.

```
$ vela install
- Installing Vela Core Chart:
Vela system along with OAM runtime already exist.
Automatically discover capabilities successfully ✅ Add(0) Update(0) Delete(8)

TYPE       	CATEGORY	DESCRIPTION
-task      	workload	One-off task to run a piece of code or script to completion
-webservice	workload	Long-running scalable service with stable endpoint to receive external traffic
-worker    	workload	Long-running scalable backend worker without network endpoint
-autoscale 	trait   	Automatically scale the app following certain triggers or metrics
-metrics   	trait   	Configure metrics targets to be monitored for the app
-rollout   	trait   	Configure canary deployment strategy to release the app
-route     	trait   	Configure route policy to the app
-scaler    	trait   	Manually scale the app

- Finished successfully.
```

And manually apply all WorkloadDefinition and TraitDefinition manifests to have all capabilities back.

```
$ kubectl apply -f charts/vela-core/templates/defwithtemplate
traitdefinition.core.oam.dev/autoscale created
traitdefinition.core.oam.dev/scaler created
traitdefinition.core.oam.dev/metrics created
traitdefinition.core.oam.dev/rollout created
traitdefinition.core.oam.dev/route created
workloaddefinition.core.oam.dev/task created
workloaddefinition.core.oam.dev/webservice created
workloaddefinition.core.oam.dev/worker created

$ vela workloads
Automatically discover capabilities successfully ✅ Add(8) Update(0) Delete(0)

TYPE       	CATEGORY	DESCRIPTION
+task      	workload	One-off task to run a piece of code or script to completion
+webservice	workload	Long-running scalable service with stable endpoint to receive external traffic
+worker    	workload	Long-running scalable backend worker without network endpoint
+autoscale 	trait   	Automatically scale the app following certain triggers or metrics
+metrics   	trait   	Configure metrics targets to be monitored for the app
+rollout   	trait   	Configure canary deployment strategy to release the app
+route     	trait   	Configure route policy to the app
+scaler    	trait   	Manually scale the app

NAME      	DESCRIPTION
task      	One-off task to run a piece of code or script to completion
webservice	Long-running scalable service with stable endpoint to receive external traffic
worker    	Long-running scalable backend worker without network endpoint
```

### Error: ScopeDefinition exists

Occasionally you might hit the issue as below. It happens when there is an old OAM Kubernetes Runtime release, or you applied `ScopeDefinition` before.

```
$ vela install
  - Installing Vela Core Chart:
  install chart vela-core, version 0.1.0, desc : A Helm chart for Kube Vela core, contains 35 file
  Failed to install the chart with error: ScopeDefinition "healthscopes.core.oam.dev" in namespace "" exists and cannot be imported into the current release: invalid ownership metadata; annotation validation error: key "meta.helm.sh/release-name" must equal "kubevela": current value is "oam"; annotation validation error: key "meta.helm.sh/release-namespace" must equal "vela-system": current value is "oam-system"
  rendered manifests contain a resource that already exists. Unable to continue with install
  helm.sh/helm/v3/pkg/action.(*Install).Run
  	/home/runner/go/pkg/mod/helm.sh/helm/v3@v3.2.4/pkg/action/install.go:274
  ...
  Error: rendered manifests contain a resource that already exists. Unable to continue with install: ScopeDefinition "healthscopes.core.oam.dev" in namespace "" exists and cannot be imported into the current release: invalid ownership metadata; annotation validation error: key "meta.helm.sh/release-name" must equal "kubevela": current value is "oam"; annotation validation error: key "meta.helm.sh/release-namespace" must equal "vela-system": current value is "oam-system"
```

Delete `ScopeDefinition` "healthscopes.core.oam.dev" and try again.

```
$ kubectl delete ScopeDefinition "healthscopes.core.oam.dev"
scopedefinition.core.oam.dev "healthscopes.core.oam.dev" deleted

$ vela install
- Installing Vela Core Chart:
install chart vela-core, version 0.1.0, desc : A Helm chart for Kube Vela core, contains 35 file
Successfully installed the chart, status: deployed, last deployed time = 2020-12-03 16:26:41.491426 +0800 CST m=+4.026069452
WARN: handle workload template `containerizedworkloads.core.oam.dev` failed: no template found, you will unable to use this workload capabilityWARN: handle trait template `manualscalertraits.core.oam.dev` failed
: no template found, you will unable to use this trait capabilityAutomatically discover capabilities successfully ✅ Add(8) Update(0) Delete(0)

TYPE       	CATEGORY	DESCRIPTION
+task      	workload	One-off task to run a piece of code or script to completion
+webservice	workload	Long-running scalable service with stable endpoint to receive external traffic
+worker    	workload	Long-running scalable backend worker without network endpoint
+autoscale 	trait   	Automatically scale the app following certain triggers or metrics
+metrics   	trait   	Configure metrics targets to be monitored for the app
+rollout   	trait   	Configure canary deployment strategy to release the app
+route     	trait   	Configure route policy to the app
+scaler    	trait   	Manually scale the app

- Finished successfully.
```

### Warning: capability rollout was not ready

If you hit the issue as below, it means cert-manager is creating a certificate for Trait Rollout.
Just wait a few minutes, and the issue will be self-healed.
                               

```
$ vela install
- Installing Vela Core Chart:
created namespace vela-system
install chart vela-core, version 0.1.0, desc : A Helm chart for Kube Vela core, contains 35 file
Successfully installed the chart, status: deployed, last deployed time = 2020-12-02 22:16:03.504685599 +0800 CST m=+4.949923921
- Vela-Core was installed successfully while some capabilities were still installing background, try running 'vela workloads' or 'vela traits' to check after a while, details timeout checking capability ready: capability 'rollout' was not ready: no matches for flagger.app/, Resource=canaries 


$ kubectl get all -n vela-system
NAME                                      READY   STATUS              RESTARTS   AGE
pod/kubevela-vela-core-77f4494f8f-hmrc8   0/1     ContainerCreating   0          13m

NAME                                 READY   UP-TO-DATE   AVAILABLE   AGE
deployment.apps/kubevela-vela-core   0/1     1            0           13m

$ kubectl describe deployment kubevela-vela-core -n vela-system
Events:
  Type     Reason       Age                         From               Message
  ----     ------       ----                        ----               -------
  Normal   Scheduled    15m                         default-scheduler  Successfully assigned vela-system/kubevela-vela-core-77f4494f8f-hmrc8 to node1
  Warning  FailedMount  <invalid>                   kubelet, node1     Unable to attach or mount volumes: unmounted volumes=[tls-cert-vol], unattached volumes=[kubevela-vela-core-token-dztdl tls-cert-vol]: timed out waiting for the condition
  Warning  FailedMount  <invalid> (x5 over 2m22s)   kubelet, node1     Unable to attach or mount volumes: unmounted volumes=[tls-cert-vol], unattached volumes=[tls-cert-vol kubevela-vela-core-token-dztdl]: timed out waiting for the condition
  Warning  FailedMount  <invalid> (x15 over 4m25s)  kubelet, node1     MountVolume.SetUp failed for volume "tls-cert-vol" : secret "webhook-server-cert" not found
```

### You have reached your pull rate limit

When you look into the logs of Pod kubevela-vela-core and found the issue as below.

```
$ kubectl get pod -n vela-system -l app.kubernetes.io/name=vela-core
NAME                                 READY   STATUS    RESTARTS   AGE
kubevela-vela-core-f8b987775-wjg25   0/1     -         0          35m
```

>Error response from daemon: toomanyrequests: You have reached your pull rate limit. You may increase the limit by 
>authenticating and upgrading: https://www.docker.com/increase-rate-limit
 
You can use github container registry instead.

```
$ docker pull ghcr.io/oam-dev/kubevela/vela-core:latest
```

### Warning: Namespace cert-manager exists

If you hit the issue as below, an `cert-manager` release might exist whose namespace and RBAC related resource conflict
with KubeVela.

```
$ vela install
- Installing Vela Core Chart:
install chart vela-core, version 0.1.0, desc : A Helm chart for Kube Vela core, contains 35 file
Failed to install the chart with error: Namespace "cert-manager" in namespace "" exists and cannot be imported into the current release: invalid ownership metadata; label validation error: missing key "app.kubernetes.io/managed-by": must be set to "Helm"; annotation validation error: missing key "meta.helm.sh/release-name": must be set to "kubevela"; annotation validation error: missing key "meta.helm.sh/release-namespace": must be set to "vela-system"
rendered manifests contain a resource that already exists. Unable to continue with install
helm.sh/helm/v3/pkg/action.(*Install).Run
        /home/runner/go/pkg/mod/helm.sh/helm/v3@v3.2.4/pkg/action/install.go:274
...
        /opt/hostedtoolcache/go/1.14.12/x64/src/runtime/asm_amd64.s:1373
Error: rendered manifests contain a resource that already exists. Unable to continue with install: Namespace "cert-manager" in namespace "" exists and cannot be imported into the current release: invalid ownership metadata; label validation error: missing key "app.kubernetes.io/managed-by": must be set to "Helm"; annotation validation error: missing key "meta.helm.sh/release-name": must be set to "kubevela"; annotation validation error: missing key "meta.helm.sh/release-namespace": must be set to "vela-system"
```

Try these steps to fix the problem.

- Delete release `cert-manager`
- Delete namespace `cert-manager`
- Install KubeVela again

```
$ helm delete cert-manager -n cert-manager
release "cert-manager" uninstalled

$ kubectl delete ns cert-manager
namespace "cert-manager" deleted

$ vela install
- Installing Vela Core Chart:
install chart vela-core, version 0.1.0, desc : A Helm chart for Kube Vela core, contains 35 file
Successfully installed the chart, status: deployed, last deployed time = 2020-12-04 10:46:46.782617 +0800 CST m=+4.248889379
Automatically discover capabilities successfully ✅ (no changes)

TYPE      	CATEGORY	DESCRIPTION
task      	workload	One-off task to run a piece of code or script to completion
webservice	workload	Long-running scalable service with stable endpoint to receive external traffic
worker    	workload	Long-running scalable backend worker without network endpoint
autoscale 	trait   	Automatically scale the app following certain triggers or metrics
metrics   	trait   	Configure metrics targets to be monitored for the app
rollout   	trait   	Configure canary deployment strategy to release the app
route     	trait   	Configure route policy to the app
scaler    	trait   	Manually scale the app
- Finished successfully.
```

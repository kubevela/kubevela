# Legacy Support

Now lots of apps are still running on Kubernetes clusters version v1.14 or v1.15, while KubeVela core requires the minimum
Kubernetes version to be v1.16+.

Currently, the main blocker is KubeVela uses CRD v1, while those old Kubernetes versions don't support CRD v1.
So we generate v1beta1 CRD here for convenience. But we have no guarantee that KubeVela core will support the
legacy Kubernetes versions. 

Follow the instructions in [README](../README.md) to create a namespace like `vela-system` and add the OAM Kubernetes
Runtime helm repo.

```
$ kubectl create namespace vela-system
$ helm repo add kubevela https://kubevelacharts.oss-cn-hangzhou.aliyuncs.com/core
```

Run the following command to install a KubeVela core legacy chart.

```
$ helm install -n vela-system vela-core-legacy kubevela/vela-core-legacy
```

If you'd like to install an older version of the legacy chart, use `helm search` to choose a proper chart version.
```
$ helm search repo vela-core-legacy --devel -l
  NAME                     	CHART VERSION	APP VERSION	DESCRIPTION
  kubevela/vela-core-legacy	0.2          	0.2        	A Helm chart for legacy KubeVela core Controlle...
  kubevela/vela-core-legacy	0.0.1        	0.1        	A Helm chart for legacy KubeVela core Controlle...

$ helm install -n vela-system kubevela-legacy kubevela/vela-core-legacy --version 0.0.1 --devel
```

Install the legacy chart as below if you want a nightly version.

```
$ helm install -n vela-system vela-core-legacy kubevela/vela-core-legacy --set image.tag=master --devel
```

---
title:  Managing Capabilities
---

In KubeVela, developers can install more capabilities (i.e. new component types and traits) from any GitHub repo that contains OAM definition files. We call these GitHub repos as _Capability Centers_. 

KubeVela is able to discover OAM definition files in this repo automatically and sync them to your own KubeVela platform.

## Add a capability center

Add and sync a capability center in KubeVela:

```bash
$ vela cap center config my-center https://github.com/oam-dev/catalog/tree/master/registry
successfully sync 1/1 from my-center remote center
Successfully configured capability center my-center and sync from remote

$ vela cap center sync my-center
successfully sync 1/1 from my-center remote center
sync finished
```

Now, this capability center `my-center` is ready to use.

## List capability centers

You are allowed to add more capability centers and list them.

```bash
$ vela cap center ls
NAME     	ADDRESS
my-center	https://github.com/oam-dev/catalog/tree/master/registry
```

## [Optional] Remove a capability center

Or, remove one.

```bash
$ vela cap center remove my-center
```

## List all available capabilities in capability center

Or, list all available capabilities in certain center.

```bash
$ vela cap ls my-center
NAME               	CENTER   	TYPE               	DEFINITION                    	STATUS     	APPLIES-TO
clonesetservice    	my-center	componentDefinition	clonesets.apps.kruise.io      	uninstalled	[]
```

## Install a capability from capability center

Now let's try to install the new component named `clonesetservice` from `my-center` to your own KubeVela platform.

You need to install OpenKruise first.

```shell
helm install kruise https://github.com/openkruise/kruise/releases/download/v0.7.0/kruise-chart.tgz
```

Install `clonesetservice` component from `my-center`.

```bash
$ vela cap install my-center/clonesetservice
Installing component capability clonesetservice
Successfully installed capability clonesetservice from my-center
```

## Use the newly installed capability

Let's check the `clonesetservice` appears in your platform firstly:

```bash
$ vela components
NAME           	NAMESPACE  	WORKLOAD                	DESCRIPTION
clonesetservice	vela-system	clonesets.apps.kruise.io	Describes long-running, scalable, containerized services
               	           	                        	that have a stable network endpoint to receive external
               	           	                        	network traffic from customers. If workload type is skipped
               	           	                        	for any service defined in Appfile, it will be defaulted to
               	           	                        	`webservice` type.
```

Great! Now let's deploy an app via Appfile.

```bash
$ cat << EOF > vela.yaml
name: testapp
services:
  testsvc:
    type: clonesetservice
    image: crccheck/hello-world
    port: 8000
EOF
```

```bash
$ vela up
Parsing vela appfile ...
Load Template ...

Rendering configs for service (testsvc)...
Writing deploy config to (.vela/deploy.yaml)

Applying application ...
Checking if app has been deployed...
App has not been deployed, creating a new deployment...
Updating:  core.oam.dev/v1alpha2, Kind=HealthScope in default
✅ App has been deployed 🚀🚀🚀
    Port forward: vela port-forward testapp
             SSH: vela exec testapp
         Logging: vela logs testapp
      App status: vela status testapp
  Service status: vela status testapp --svc testsvc
```

then you can Get a cloneset in your environment.

```shell
$ kubectl get clonesets.apps.kruise.io
NAME      DESIRED   UPDATED   UPDATED_READY   READY   TOTAL   AGE
testsvc   1         1         1               1       1       46s
```

## Uninstall a capability

> NOTE: make sure no apps are using the capability before uninstalling.

```bash
$ vela cap uninstall my-center/clonesetservice
Successfully uninstalled capability clonesetservice
```

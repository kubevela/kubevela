---
title:  Debug, Test and Dry-run
---

With flexibility in defining abstractions, it's important to be able to debug, test and dry-run the CUE based definitions. This tutorial will show this step by step.

## Prerequisites

Please make sure below CLIs are present in your environment:
* [`cue` >=v0.2.2](https://cuelang.org/docs/install/)
* [`vela` (>v1.0.0)](../install#4-optional-get-kubevela-cli)


## Define Definition and Template

We recommend to define the `Definition Object` in two separate parts: the CRD part and the CUE template. This enable us to debug, test and dry-run the CUE template.

Let's name the CRD part as `def.yaml`.

```yaml
apiVersion: core.oam.dev/v1beta1
kind: ComponentDefinition
metadata:
  name: microservice
  annotations:
    definition.oam.dev/description: "Describes a microservice combo Deployment with Service."
spec:
  workload:
    definition:
      apiVersion: apps/v1
      kind: Deployment
  schematic:
    cue:
      template: |
```

And the CUE template part as `def.cue`, then we can use CUE commands such as `cue fmt` / `cue vet`  to format and validate the CUE file.

```
output: {
	// Deployment
	apiVersion: "apps/v1"
	kind:       "Deployment"
	metadata: {
		name:      context.name
		namespace: "default"
	}
	spec: {
		selector: matchLabels: {
			"app": context.name
		}
		template: {
			metadata: {
				labels: {
					"app":     context.name
					"version": parameter.version
				}
			}
			spec: {
				serviceAccountName:            "default"
				terminationGracePeriodSeconds: parameter.podShutdownGraceSeconds
				containers: [{
					name:  context.name
					image: parameter.image
					ports: [{
						if parameter.containerPort != _|_ {
							containerPort: parameter.containerPort
						}
						if parameter.containerPort == _|_ {
							containerPort: parameter.servicePort
						}
					}]
					if parameter.env != _|_ {
						env: [
							for k, v in parameter.env {
								name:  k
								value: v
							},
						]
					}
					resources: {
						requests: {
							if parameter.cpu != _|_ {
								cpu: parameter.cpu
							}
							if parameter.memory != _|_ {
								memory: parameter.memory
							}
						}
					}
				}]
			}
		}
	}
}
// Service
outputs: service: {
	apiVersion: "v1"
	kind:       "Service"
	metadata: {
		name: context.name
		labels: {
			"app": context.name
		}
	}
	spec: {
		type: "ClusterIP"
		selector: {
			"app": context.name
		}
		ports: [{
			port: parameter.servicePort
			if parameter.containerPort != _|_ {
				targetPort: parameter.containerPort
			}
			if parameter.containerPort == _|_ {
				targetPort: parameter.servicePort
			}
		}]
	}
}
parameter: {
	version:        *"v1" | string
	image:          string
	servicePort:    int
	containerPort?: int
	// +usage=Optional duration in seconds the pod needs to terminate gracefully
	podShutdownGraceSeconds: *30 | int
	env: [string]: string
	cpu?:    string
	memory?: string
}
```

After everything is done, there's a script [`hack/vela-templates/mergedef.sh`](https://github.com/oam-dev/kubevela/blob/master/hack/vela-templates/mergedef.sh) to merge the `def.yaml` and `def.cue` into a completed Definition Object.

```shell
$ ./hack/vela-templates/mergedef.sh def.yaml def.cue > microservice-def.yaml
```

## Debug CUE template

### Use `cue vet` to Validate

```shell
$ cue vet def.cue
output.metadata.name: reference "context" not found:
    ./def.cue:6:14
output.spec.selector.matchLabels.app: reference "context" not found:
    ./def.cue:11:11
output.spec.template.metadata.labels.app: reference "context" not found:
    ./def.cue:16:17
output.spec.template.spec.containers.name: reference "context" not found:
    ./def.cue:24:13
outputs.service.metadata.name: reference "context" not found:
    ./def.cue:62:9
outputs.service.metadata.labels.app: reference "context" not found:
    ./def.cue:64:11
outputs.service.spec.selector.app: reference "context" not found:
    ./def.cue:70:11
```

The `reference "context" not found` is a common error in this step as [`context`](/docs/cue/component?id=cue-context) is a runtime information that only exist in KubeVela controllers. In order to validate the CUE template end-to-end, we can add a mock `context` in `def.cue`.

> Note that you need to remove all mock data when you finished the validation.

```CUE
... // existing template data
context: {
    name: string
}
```

Then execute the command:

```shell
$ cue vet def.cue
some instances are incomplete; use the -c flag to show errors or suppress this message
```

The `reference "context" not found` error is gone, but  `cue vet` only validates the data type which is not enough to ensure the login in template is correct. Hence we need to use `cue vet -c` for complete validation:

```shell
$ cue vet def.cue -c
context.name: incomplete value string
output.metadata.name: incomplete value string
output.spec.selector.matchLabels.app: incomplete value string
output.spec.template.metadata.labels.app: incomplete value string
output.spec.template.spec.containers.0.image: incomplete value string
output.spec.template.spec.containers.0.name: incomplete value string
output.spec.template.spec.containers.0.ports.0.containerPort: incomplete value int
outputs.service.metadata.labels.app: incomplete value string
outputs.service.metadata.name: incomplete value string
outputs.service.spec.ports.0.port: incomplete value int
outputs.service.spec.ports.0.targetPort: incomplete value int
outputs.service.spec.selector.app: incomplete value string
parameter.image: incomplete value string
parameter.servicePort: incomplete value int
```

It now complains some runtime data is incomplete (because `context` and `parameter` do not have value), let's now fill in more mock data in the `def.cue` file:

```CUE
context: {
	name: "test-app"
}
parameter: {
	version:       "v2"
	image:         "image-address"
	servicePort:   80
	containerPort: 8000
	env: {"PORT": "8000"}
	cpu:    "500m"
	memory: "128Mi"
}
```

It won't complain now which means validation is passed:

```shell
cue vet def.cue -c
```

#### Use `cue export` to Check the Rendered Resources

The `cue export` can export rendered result in YAMl foramt:

```shell
$ cue export -e output def.cue --out yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: test-app
  namespace: default
spec:
  selector:
    matchLabels:
      app: test-app
  template:
    metadata:
      labels:
        app: test-app
        version: v2
    spec:
      serviceAccountName: default
      terminationGracePeriodSeconds: 30
      containers:
        - name: test-app
          image: image-address
```

```shell
$ cue export -e outputs.service def.cue --out yaml
apiVersion: v1
kind: Service
metadata:
  name: test-app
  labels:
    app: test-app
spec:
  selector:
    app: test-app
  type: ClusterIP
```

### Test CUE Template with `Kube` package

KubeVela automatically generates internal CUE packages for all built-in Kubernetes API resources including CRDs.
You can import them in CUE template to simplify your templates and help you do the validation.

There are two kinds of ways to import internal `kube` packages.

1. Import them with fixed style: `kube/<apiVersion>` and using it by `Kind`.
    ```cue
    import (
     apps "kube/apps/v1"
     corev1 "kube/v1"
    )
    // output is validated by Deployment.
    output: apps.#Deployment
    outputs: service: corev1.#Service
   ```
   This way is very easy to remember and use because it aligns with the K8s Object usage, only need to add a prefix `kube/` before `apiVersion`.
   While this way only supported in KubeVela, so you can only debug and test it with [`vela system dry-run`](#dry-run-the-application).
   
2. Import them with third-party packages style. You can run `vela system cue-packages` to list all build-in `kube` packages
   to know the `third-party packages` supported currently.
    ```shell
    $ vela system cue-packages
    DEFINITION-NAME                	IMPORT-PATH                         	 USAGE
    #Deployment                    	k8s.io/apps/v1                      	Kube Object for apps/v1.Deployment
    #Service                       	k8s.io/core/v1                      	Kube Object for v1.Service
    #Secret                        	k8s.io/core/v1                      	Kube Object for v1.Secret
    #Node                          	k8s.io/core/v1                      	Kube Object for v1.Node
    #PersistentVolume              	k8s.io/core/v1                      	Kube Object for v1.PersistentVolume
    #Endpoints                     	k8s.io/core/v1                      	Kube Object for v1.Endpoints
    #Pod                           	k8s.io/core/v1                      	Kube Object for v1.Pod
    ```
   In fact, they are all built-in packages, but you can import them with the `import-path` like the `third-party packages`.
   In this way, you could debug with `cue` cli client.
   

#### A workflow to debug with `kube` packages

Here's a workflow that you can debug and test the CUE template with `cue` CLI and use **exactly the same CUE template** in KubeVela.

1. Create a test directory, Init CUE modules.

```shell
mkdir cue-debug && cd cue-debug/
cue mod init oam.dev
go mod init oam.dev
touch def.cue
```

2. Download the `third-party packages` by using `cue` CLI.

In KubeVela, we don't need to download these packages as they're automatically generated from K8s API.
But for local test, we need to use `cue get go` to fetch Go packages and convert them to CUE format files.

So, by using K8s `Deployment` and `Serivice`, we need download and convert to CUE definitions for the `core` and `apps` Kubernetes modules like below:

```shell
cue get go k8s.io/api/core/v1
cue get go k8s.io/api/apps/v1
```

After that, the module directory will show the following contents:

```shell
├── cue.mod
│   ├── gen
│   │   └── k8s.io
│   │       ├── api
│   │       │   ├── apps
│   │       │   └── core
│   │       └── apimachinery
│   │           └── pkg
│   ├── module.cue
│   ├── pkg
│   └── usr
├── def.cue
├── go.mod
└── go.sum
```

The package import path in CUE template should be:

```cue
import (
   apps "k8s.io/api/apps/v1"
   corev1 "k8s.io/api/core/v1"
)
```

3. Refactor directory hierarchy.

Our goal is to test template locally and use the same template in KubeVela.
So we need to refactor our local CUE module directories a bit to align with the import path provided by KubeVela,

Copy the `apps` and `core` from `cue.mod/gen/k8s.io/api` to `cue.mod/gen/k8s.io`.
(Note we should keep the source directory `apps` and `core` in `gen/k8s.io/api` to avoid package dependency issues).

```bash
cp -r cue.mod/gen/k8s.io/api/apps cue.mod/gen/k8s.io
cp -r cue.mod/gen/k8s.io/api/core cue.mod/gen/k8s.io
```

The modified module directory should like:

```shell
├── cue.mod
│   ├── gen
│   │   └── k8s.io
│   │       ├── api
│   │       │   ├── apps
│   │       │   └── core
│   │       ├── apimachinery
│   │       │   └── pkg
│   │       ├── apps
│   │       └── core
│   ├── module.cue
│   ├── pkg
│   └── usr
├── def.cue
├── go.mod
└── go.sum
```

So, you can import the package use the following path that aligns with KubeVela:

```cue
import (
   apps "k8s.io/apps/v1"
   corev1 "k8s.io/core/v1"
)
```

4. Test and Run.

Finally, we can test CUE Template which use the `Kube` package.

```cue
import (
   apps "k8s.io/apps/v1"
   corev1 "k8s.io/core/v1"
)

// output is validated by Deployment.
output: apps.#Deployment
output: {
	metadata: {
		name:      context.name
		namespace: "default"
	}
	spec: {
		selector: matchLabels: {
			"app": context.name
		}
		template: {
			metadata: {
				labels: {
					"app":     context.name
					"version": parameter.version
				}
			}
			spec: {
				terminationGracePeriodSeconds: parameter.podShutdownGraceSeconds
				containers: [{
					name:  context.name
					image: parameter.image
					ports: [{
						if parameter.containerPort != _|_ {
							containerPort: parameter.containerPort
						}
						if parameter.containerPort == _|_ {
							containerPort: parameter.servicePort
						}
					}]
					if parameter.env != _|_ {
						env: [
							for k, v in parameter.env {
								name:  k
								value: v
							},
						]
					}
					resources: {
						requests: {
							if parameter.cpu != _|_ {
								cpu: parameter.cpu
							}
							if parameter.memory != _|_ {
								memory: parameter.memory
							}
						}
					}
				}]
			}
		}
	}
}

outputs:{
  service: corev1.#Service
}


// Service
outputs: service: {
	metadata: {
		name: context.name
		labels: {
			"app": context.name
		}
	}
	spec: {
		//type: "ClusterIP"
		selector: {
			"app": context.name
		}
		ports: [{
			port: parameter.servicePort
			if parameter.containerPort != _|_ {
				targetPort: parameter.containerPort
			}
			if parameter.containerPort == _|_ {
				targetPort: parameter.servicePort
			}
		}]
	}
}
parameter: {
	version:        *"v1" | string
	image:          string
	servicePort:    int
	containerPort?: int
	// +usage=Optional duration in seconds the pod needs to terminate gracefully
	podShutdownGraceSeconds: *30 | int
	env: [string]: string
	cpu?:    string
	memory?: string
}

// mock context data
context: {
    name: "test"
}

// mock parameter data
parameter: {
	image:          "test-image"
	servicePort:    8000
	env: {
        "HELLO": "WORLD"
    }
}
```

Use `cue export` to see the export result.

```shell
$ cue export def.cue --out yaml
output:
  metadata:
    name: test
    namespace: default
  spec:
    selector:
      matchLabels:
        app: test
    template:
      metadata:
        labels:
          app: test
          version: v1
      spec:
        terminationGracePeriodSeconds: 30
        containers:
        - name: test
          image: test-image
          ports:
          - containerPort: 8000
          env:
          - name: HELLO
            value: WORLD
          resources:
            requests: {}
outputs:
  service:
    metadata:
      name: test
      labels:
        app: test
    spec:
      selector:
        app: test
      ports:
      - port: 8000
        targetPort: 8000
parameter:
  version: v1
  image: test-image
  servicePort: 8000
  podShutdownGraceSeconds: 30
  env:
    HELLO: WORLD
context:
  name: test
```

## Dry-Run the `Application`

When CUE template is good, we can use `vela system dry-run` to dry run and check the rendered resources in real Kubernetes cluster. This command will exactly execute the same render logic in KubeVela's `Application` Controller and output the result for you.

First, we need use `mergedef.sh` to merge the definition and cue files.

```shell
$ mergedef.sh def.yaml def.cue > componentdef.yaml
```

Then, let's create an Application named `test-app.yaml`.

```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: boutique
  namespace: default
spec:
  components:
    - name: frontend
      type: microservice
      properties:
        image: registry.cn-hangzhou.aliyuncs.com/vela-samples/frontend:v0.2.2
        servicePort: 80
        containerPort: 8080
        env:
          PORT: "8080"
        cpu: "100m"
        memory: "64Mi"
```

Dry run the application by using `vela system dry-run`.

```shell
$ vela system dry-run -f test-app.yaml -d componentdef.yaml
---
# Application(boutique) -- Comopnent(frontend)
---

apiVersion: apps/v1
kind: Deployment
metadata:
  labels:
    app.oam.dev/component: frontend
    app.oam.dev/name: boutique
    workload.oam.dev/type: microservice
  name: frontend
  namespace: default
spec:
  selector:
    matchLabels:
      app: frontend
  template:
    metadata:
      labels:
        app: frontend
        version: v1
    spec:
      containers:
      - env:
        - name: PORT
          value: "8080"
        image: registry.cn-hangzhou.aliyuncs.com/vela-samples/frontend:v0.2.2
        name: frontend
        ports:
        - containerPort: 8080
        resources:
          requests:
            cpu: 100m
            memory: 64Mi
      serviceAccountName: default
      terminationGracePeriodSeconds: 30

---
apiVersion: v1
kind: Service
metadata:
  labels:
    app: frontend
    app.oam.dev/component: frontend
    app.oam.dev/name: boutique
    trait.oam.dev/resource: service
    trait.oam.dev/type: AuxiliaryWorkload
  name: frontend
spec:
  ports:
  - port: 80
    targetPort: 8080
  selector:
    app: frontend
  type: ClusterIP

---
```

`-d` or `--definitions` is a useful flag permitting user to provide capability
definitions used in the application from local files.
`dry-run` cmd will prioritize the provided capabilities than the living
ones in the cluster.
If the capability is not found in local files and cluster, it will raise an error.

## Live-Diff the `Application`

`vela system live-diff` allows users to have a preview of what would change if
upgrade an application.
It basically generates a diff between the specific revision of an application 
and the result of `vela system dry-run`.
The result shows the changes (added/modified/removed/no_change) of the application as well as its sub-resources, such as components and traits.
`live-diff` will not make any changes to the living cluster, so it's very
helpful if you want to update an application but worry about the unknown results
that may be produced.

Let's prepare an application and deploy it.

> ComponentDefinitions and TraitDefinitions used in this sample are stored in 
`./doc/examples/live-diff/definitions`.

```yaml
# app.yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: livediff-demo
spec:
  components:
    - name: myweb-1
      type: myworker
      properties:
        image: "busybox"
        cmd:
          - sleep
          - "1000"
        lives: "3"
        enemies: "alien"
      traits:
        - type: myingress
          properties:
            domain: "www.example.com"
            http:
              "/": 80
        - type: myscaler
          properties:
            replicas: 2
    - name: myweb-2
      type: myworker
      properties:
        image: "busybox"
        cmd:
          - sleep
          - "1000"
        lives: "3"
        enemies: "alien"
```

```shell
kubectl apply ./doc/examples/live-diff/definitions
kubectl apply ./doc/examples/live-diff/app.yaml
```

Then, assume we want to update the application with below configuration.
To preview changes brought by updating while not really apply updated
configuration into the cluster, we can use `live-diff` here.

```yaml
# app-updated.yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: livediff-demo
spec:
  components:
    - name: myweb-1
      type: myworker
      properties:
        image: "busybox"
        cmd:
          - sleep
          - "2000" # change a component property
        lives: "3"
        enemies: "alien"
      traits:
        - type: myingress
          properties:
            domain: "www.example.com"
            http:
              "/": 90 # change a trait
        # - type: myscaler # remove a trait
        #   properties:
        #     replicas: 2
    - name: myweb-2
      type: myworker
      properties: # no change on component property
        image: "busybox"
        cmd:
          - sleep
          - "1000"
        lives: "3"
        enemies: "alien"
      traits:
        - type: myingress # add a trait
          properties:
            domain: "www.example.com"
            http:
              "/": 90
    - name: myweb-3  # add a component
      type: myworker
      properties:
        image: "busybox"
        cmd:
          - sleep
          - "1000"
        lives: "3"
        enemies: "alien"
      traits:
        - type: myingress
          properties:
            domain: "www.example.com"
            http:
              "/": 90
```

```shell
vela system live-diff -f  ./doc/examples/live-diff/app-modified.yaml -r livediff-demo-v1
```

`-r` or `--revision` is a flag that specifies the name of a living
`ApplicationRevision` with which you want to compare the updated application.

`-c` or `--context` is a flag that specifies the number of lines shown around a
change.
The unchanged lines which are out of the context of a change will be omitted.
It's useful if the diff result contains a lot of unchanged content while
you just want to focus on the changed ones.

<details><summary> Click to view diff result </summary>

```shell
---
# Application (application-sample) has been modified(*)
---
  apiVersion: core.oam.dev/v1beta1
  kind: Application
  metadata:
    creationTimestamp: null
-   name: application-sample
+   name: livediff-demo
    namespace: default
  spec:
    components:
    - name: myweb-1
+     properties:
+       cmd:
+       - sleep
+       - "2000"
+       enemies: alien
+       image: busybox
+       lives: "3"
+     traits:
+     - properties:
+         domain: www.example.com
+         http:
+           /: 90
+       type: myingress
+     type: myworker
+   - name: myweb-2
      properties:
        cmd:
        - sleep
        - "1000"
        enemies: alien
        image: busybox
        lives: "3"
      traits:
      - properties:
          domain: www.example.com
          http:
-           /: 80
+           /: 90
        type: myingress
-     - properties:
-         replicas: 2
-       type: myscaler
      type: myworker
-   - name: myweb-2
+   - name: myweb-3
      properties:
        cmd:
        - sleep
        - "1000"
        enemies: alien
        image: busybox
        lives: "3"
+     traits:
+     - properties:
+         domain: www.example.com
+         http:
+           /: 90
+       type: myingress
      type: myworker
  status:
    batchRollingState: ""
    currentBatch: 0
    rollingState: ""
    upgradedReadyReplicas: 0
    upgradedReplicas: 0
  
---
## Component (myweb-1) has been modified(*)
---
  apiVersion: core.oam.dev/v1alpha2
  kind: Component
  metadata:
    creationTimestamp: null
    labels:
-     app.oam.dev/name: application-sample
+     app.oam.dev/name: livediff-demo
    name: myweb-1
  spec:
    workload:
      apiVersion: apps/v1
      kind: Deployment
      metadata:
        labels:
          app.oam.dev/appRevision: ""
          app.oam.dev/component: myweb-1
-         app.oam.dev/name: application-sample
+         app.oam.dev/name: livediff-demo
          workload.oam.dev/type: myworker
      spec:
        selector:
          matchLabels:
            app.oam.dev/component: myweb-1
        template:
          metadata:
            labels:
              app.oam.dev/component: myweb-1
          spec:
            containers:
            - command:
              - sleep
-             - "1000"
+             - "2000"
              image: busybox
              name: myweb-1
  status:
    observedGeneration: 0
  
---
### Component (myweb-1) / Trait (myingress/ingress) has been modified(*)
---
  apiVersion: networking.k8s.io/v1beta1
  kind: Ingress
  metadata:
    labels:
      app.oam.dev/appRevision: ""
      app.oam.dev/component: myweb-1
-     app.oam.dev/name: application-sample
+     app.oam.dev/name: livediff-demo
      trait.oam.dev/resource: ingress
      trait.oam.dev/type: myingress
    name: myweb-1
  spec:
    rules:
    - host: www.example.com
      http:
        paths:
        - backend:
            serviceName: myweb-1
-           servicePort: 80
+           servicePort: 90
          path: /
  
---
### Component (myweb-1) / Trait (myingress/service) has been modified(*)
---
  apiVersion: v1
  kind: Service
  metadata:
    labels:
      app.oam.dev/appRevision: ""
      app.oam.dev/component: myweb-1
-     app.oam.dev/name: application-sample
+     app.oam.dev/name: livediff-demo
      trait.oam.dev/resource: service
      trait.oam.dev/type: myingress
    name: myweb-1
  spec:
    ports:
-   - port: 80
-     targetPort: 80
+   - port: 90
+     targetPort: 90
    selector:
      app.oam.dev/component: myweb-1
  
---
### Component (myweb-1) / Trait (myscaler/scaler) has been removed(-)
---
- apiVersion: core.oam.dev/v1alpha2
- kind: ManualScalerTrait
- metadata:
-   labels:
-     app.oam.dev/appRevision: ""
-     app.oam.dev/component: myweb-1
-     app.oam.dev/name: application-sample
-     trait.oam.dev/resource: scaler
-     trait.oam.dev/type: myscaler
- spec:
-   replicaCount: 2
  
---
## Component (myweb-2) has been modified(*)
---
  apiVersion: core.oam.dev/v1alpha2
  kind: Component
  metadata:
    creationTimestamp: null
    labels:
-     app.oam.dev/name: application-sample
+     app.oam.dev/name: livediff-demo
    name: myweb-2
  spec:
    workload:
      apiVersion: apps/v1
      kind: Deployment
      metadata:
        labels:
          app.oam.dev/appRevision: ""
          app.oam.dev/component: myweb-2
-         app.oam.dev/name: application-sample
+         app.oam.dev/name: livediff-demo
          workload.oam.dev/type: myworker
      spec:
        selector:
          matchLabels:
            app.oam.dev/component: myweb-2
        template:
          metadata:
            labels:
              app.oam.dev/component: myweb-2
          spec:
            containers:
            - command:
              - sleep
              - "1000"
              image: busybox
              name: myweb-2
  status:
    observedGeneration: 0
  
---
### Component (myweb-2) / Trait (myingress/ingress) has been added(+)
---
+ apiVersion: networking.k8s.io/v1beta1
+ kind: Ingress
+ metadata:
+   labels:
+     app.oam.dev/appRevision: ""
+     app.oam.dev/component: myweb-2
+     app.oam.dev/name: livediff-demo
+     trait.oam.dev/resource: ingress
+     trait.oam.dev/type: myingress
+   name: myweb-2
+ spec:
+   rules:
+   - host: www.example.com
+     http:
+       paths:
+       - backend:
+           serviceName: myweb-2
+           servicePort: 90
+         path: /
  
---
### Component (myweb-2) / Trait (myingress/service) has been added(+)
---
+ apiVersion: v1
+ kind: Service
+ metadata:
+   labels:
+     app.oam.dev/appRevision: ""
+     app.oam.dev/component: myweb-2
+     app.oam.dev/name: livediff-demo
+     trait.oam.dev/resource: service
+     trait.oam.dev/type: myingress
+   name: myweb-2
+ spec:
+   ports:
+   - port: 90
+     targetPort: 90
+   selector:
+     app.oam.dev/component: myweb-2
  
---
## Component (myweb-3) has been added(+)
---
+ apiVersion: core.oam.dev/v1alpha2
+ kind: Component
+ metadata:
+   creationTimestamp: null
+   labels:
+     app.oam.dev/name: livediff-demo
+   name: myweb-3
+ spec:
+   workload:
+     apiVersion: apps/v1
+     kind: Deployment
+     metadata:
+       labels:
+         app.oam.dev/appRevision: ""
+         app.oam.dev/component: myweb-3
+         app.oam.dev/name: livediff-demo
+         workload.oam.dev/type: myworker
+     spec:
+       selector:
+         matchLabels:
+           app.oam.dev/component: myweb-3
+       template:
+         metadata:
+           labels:
+             app.oam.dev/component: myweb-3
+         spec:
+           containers:
+           - command:
+             - sleep
+             - "1000"
+             image: busybox
+             name: myweb-3
+ status:
+   observedGeneration: 0
  
---
### Component (myweb-3) / Trait (myingress/ingress) has been added(+)
---
+ apiVersion: networking.k8s.io/v1beta1
+ kind: Ingress
+ metadata:
+   labels:
+     app.oam.dev/appRevision: ""
+     app.oam.dev/component: myweb-3
+     app.oam.dev/name: livediff-demo
+     trait.oam.dev/resource: ingress
+     trait.oam.dev/type: myingress
+   name: myweb-3
+ spec:
+   rules:
+   - host: www.example.com
+     http:
+       paths:
+       - backend:
+           serviceName: myweb-3
+           servicePort: 90
+         path: /
  
---
### Component (myweb-3) / Trait (myingress/service) has been added(+)
---
+ apiVersion: v1
+ kind: Service
+ metadata:
+   labels:
+     app.oam.dev/appRevision: ""
+     app.oam.dev/component: myweb-3
+     app.oam.dev/name: livediff-demo
+     trait.oam.dev/resource: service
+     trait.oam.dev/type: myingress
+   name: myweb-3
+ spec:
+   ports:
+   - port: 90
+     targetPort: 90
+   selector:
+     app.oam.dev/component: myweb-3
```

</details>

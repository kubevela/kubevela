# Test and Debug CUE Templates

This documentation explains how to test and debug CUE templates using CUE CLI as well as
dry-run your capability definitions via KubeVela CLI.

## Prerequisites

* [`cue` >=v0.2.2](https://cuelang.org/docs/install/)
* [`vela` (>v1.0.0)](https://kubevela.io/#/en/install?id=_3-optional-get-kubevela-cli)

## CUE CLI basic

Below is the basic CUE data, you can define both schema and value in the same file with the almost same format:

```
a: 1.5
a: float
b: 1
b: int
d: [1, 2, 3]
g: {
	h: "abc"
}
e: string
```

Let's write them in a file named `first.cue`.

* Format the CUE file. If you're using Goland or similar JetBrains IDE,
  you can [configure save on format](https://wonderflow.info/posts/2020-11-02-goland-cuelang-format/) instead.
  This command will not only format the CUE, but also point out the wrong schema. That's very useful.
    ```shell
    cue fmt first.cue
    ```

* Schema Check, besides `cue fmt`, you can also use `vue vet` to check schema.
    ```shell
    cue vet first.cue
    ```
  
* Calculate/Render the result. `cue eval` will calculate the CUE file and render out the result.
  You can see the results don't contain `a: float` and `b: int`, because these two variables are calculated.
  While the `e: string` doesn't have definitive results, so it keeps as it is.
    ```shell
   $ cue eval first.cue
    a: 1.5
    b: 1
    d: [1, 2, 3]
    g: {
    h: "abc"
    }
    e: string
    ```
  
* Render for specified result. For example, we want only know the result of `b` in the file, then we can specify the parameter `-e`.
    ```shell
    $ cue eval -e b first.cue
    1
    ```

* Export the result. `cue export` will export the result with final value. It will report an error if some variables are not definitive. 
    ```shell
    $ cue export first.cue
    e: cannot convert incomplete value "string" to JSON:
        ./first.cue:9:4
    ```
  We can complete the value by giving a value to `e`, for example:
    ```shell
    echo "e: \"abc\"" >> first.cue
    ```
  Then, the command will work. By default, the result will be rendered in json format. 
    ```shell
    $ cue export first.cue
    {
        "a": 1.5,
        "b": 1,
        "d": [
            1,
            2,
            3
        ],
        "g": {
            "h": "abc"
        },
        "e": "abc"
    }
    ```

* Export the result in YAML format.
    ```shell
    $ cue export first.cue --out yaml
    a: 1.5
    b: 1
    d:
    - 1
    - 2
    - 3
    g:
      h: abc
    e: abc
    ```

* Export the result for specified variable.
    ```shell
    $ cue export -e g first.cue
    {
        "h": "abc"
    }
    ```

For now, you have learned all useful CUE cli operations.

## CUE language basic

* Data structure: Below is the basic data structure of CUE.

```shell
// float
a: 1.5

// int
b: 1

// string
c: "blahblahblah"

// array
d: [1, 2, 3, 1, 2, 3, 1, 2, 3]

// bool
e: true

// struct
f: {
	a: 1.5
	b: 1
	d: [1, 2, 3, 1, 2, 3, 1, 2, 3]
	g: {
		h: "abc"
	}
}

// null
j: null
```

* Define a custom CUE type. You can use a `#` symbol to specify some variable represents a CUE type.

```
#abc: string
```

Let's name it `second.cue`. Then the `cue export` won't complain as the `#abc` is a type not incomplete value.

```shell
$ cue export second.cue
{}
```

You can also define a more complex custom struct, such as:

```
#abc: {
  x: int
  y: string
  z: {
    a: float
    b: bool
  }
}
```

It's widely used in KubeVela to define templates and do validation.

* Operator  `|`, it represents a value could be both case. Below is an example that the variable `a` could be in string or int type.

```shell
a: string | int
```

* Default Value, we can use `*` symbol to represent a default value for variable. That's usually used with `|`,
  which represents a default value for some type. Below is an example that variable `a` is `int` and it's default value is `1`.

```shell
a: *1 | int
```

* Optional Variable. In some cases, a variable could not be used, they're optional variables, we can use `?:` to define it.
  In the below example, `a` is an optional variable, `x` and `z` in `#my` is optional while `y` is a required variable.

```
a ?: int

#my: {
x ?: string
y : int
z ?:float
}
```

* Operator  `&`, it used to calculate two variables.

```shell
a: *1 | int
b: 3
c: a & b
```

Saving it in `third.cue` file.

You can evaluate the result by using `cue eval`:

```shell
$ cue eval third.cue
a: 1
b: 3
c: 3
```

* Conditional statement, it's really useful when you have some cascade operations that different value affects different results.

```shell
price: number
feel: *"good" | string
// Feel bad if price is too high
if price > 100 {
    feel: "bad"
}
price: 200
```

Saving it in `fourth.cue` file.

You can evaluate the result by using `cue eval`:

```shell
$ cue eval fourth.cue
price: 200
feel:  "bad"
```

* For Loop: if you want to avoid duplicate, you may want to use for loop. 

```
#a: {
	"hello": "Barcelona"
	"nihao": "Shanghai"
}

for k, v in #a {
	"\(k)": {
		nameLen: len(v)
		value:   v
	}
}
```

Note that we use `"\( _my-statement_ )"` for inner calculation in string.

For now, you have finished learning all CUE language basic.

## CUE templating and reference

Let's try to define a CUE template with the knowledge just learned.

1. Define a custom CUE type.

```
#Config: {
	name:  string
	value: string
}
```

Let's save it in a file called `deployment.cue`.

2. Define a variable named `parameter`, and use the custom CUE type `#Config`. Using `...` in a list means the
   list can be appended with multiple elements. If we don't add `...`, then `[#Config]` means the list can only have one element in it.

```shell
parameter: {
	name: string
	image: string
	config: [...#Config]
}
```

Append it into the `deployment.cue`.


3. Define a more complex `template` variable and reference the variable `parameter`. 

```
template: {
	apiVersion: "apps/v1"
	kind:       "Deployment"
	spec: {
		selector: matchLabels: {
			"app.oam.dev/component": parameter.name
		}
		template: {
			metadata: labels: {
				"app.oam.dev/component": parameter.name
			}
			spec: {
				containers: [{
					name:  parameter.name
					image: parameter.image
					if parameter["config"] != _|_ {
						env: parameter.config
					}
				}]
			}}}
}
```

People who are familiar with Kubernetes may have understood that is a template of K8s Deployment. The `parameter` part
is the parameters of the template.

Append it into the `deployment.cue`.

4. Then, let's append the value by:  

```
parameter:{
   name: "mytest"
   image: "nginx:v1"
   config: [{name:"a",value:"b"}]
}
```

5. Finally, let's export it in yaml:

```shell
$ cue export deployment.cue -e template --out yaml
apiVersion: apps/v1
kind: Deployment
spec:
  template:
    spec:
      containers:
      - name: mytest
        env:
        - name: a
          value: b
        image: nginx:v1
    metadata:
      labels:
        app.oam.dev/component: mytest
  selector:
    matchLabels:
      app.oam.dev/component: mytest
```


## A Full Workflow

Before reading this part, please make sure you've learned [the definition and template concepts](../platform-engineers/definition-and-templates.md).
This section will guide you some useful tips to write a definition in CUE.

### Combine Definition File

Usually we define the Definition file in two parts, one is the yaml part and the other is the CUE part.

Let's name the yaml part as `def.yaml`.

```yaml
apiVersion: core.oam.dev/v1alpha2
kind: WorkloadDefinition
metadata:
  name: microservice
  annotations:
    definition.oam.dev/description: "Describes a microservice combo Deployment with Service."
spec:
  definitionRef:
    name: deployment.apps
  schematic:
    cue:
      template: |
```

And the CUE Template part as `def.cue`, then we can use `cue fmt` / `cue vet`  to format and validate the CUE file.

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

And finally there's a script [`hack/vela-templates/mergedef.sh`](https://github.com/oam-dev/kubevela/blob/master/hack/vela-templates/mergedef.sh)
can merge the `def.yaml` and `def.cue` to a completed Definition.

```shell
$ ./hack/vela-templates/mergedef.sh def.yaml def.cue > componentdef.yaml
```

### Debug CUE template

#### use `cue vet` to validate

The `cue vet` validates CUE files well.

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

The `reference "context" not found` is a very common error, this is because the [`context`](workload-type.md#context) is
a KubeVela inner variable that will be existed in runtime.

But in order to check the correctness of the CUE Template more conveniently. We can add a fake `context` in `def.cue` for test.

Note that you need to remove it when you have finished the development and test.

```CUE
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
                    ...
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
        ...
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
context: {
    name: string
}
```

Then execute the command:

```shell
$ cue vet def.cue
some instances are incomplete; use the -c flag to show errors or suppress this message
```

`cue vet` will only validates the data type. The `-c` validates that all regular fields are concrete.
We can fill in the concrete data to verify the correctness of the template.

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

Again, use the mock data for the `context` and `parameter`, append these following data in your `def.cue` file.

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

The `cue` will verify the field type in the mock parameter.
You can try any data you want until the following command is executed without complains.

```shell
cue vet def.cue -c
```

#### use `cue export` to check the result

`cue export` can export the result in yaml. It's help you to check the correctness of template with the specified output result.

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


## Dry-Run Application

After we test the CUE Template well, we can use `vela system dry-run` to dry run an application and test in in real K8s environment.
This command will show you the real k8s resources that will be created.

First, we need use `mergedef.sh` to merge the definition and cue files.

```shell
$ mergedef.sh def.yaml def.cue > componentdef.yaml
```

Then, let's create an Application named `test-app.yaml`.

```yaml
apiVersion: core.oam.dev/v1alpha2
kind: Application
metadata:
  name: boutique
  namespace: default
spec:
  components:
    - name: frontend
      type: microservice
      settings:
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

> Note: `vela system dry-run` will execute the same logic of `Application` controller in KubeVela.
> Hence it's helpful for you to test or debug.

### Import Kube Package

KubeVela automatically generates internal packages for all built-in K8s API resources based on K8s OpenAPI.

With the help of `vela system dry-run`, you can use the `import kube package` feature and test it locally.

So some default values in our `def.cue` can be simplified, and the imported package will help you validate the template:

```cue
import (
   apps "kube/apps/v1"
   corev1 "kube/v1"
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
```

Then merge them.

```shell
mergedef.sh def.yaml def.cue > componentdef.yaml
```

And dry run.

```shell
vela system dry-run -f test-app.yaml -d componentdef.yaml
```
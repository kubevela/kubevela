# CUE Basic

KubeVela use [CUE](https://cuelang.org/) as its template DSL. With the help of CUE, we can define simple but powerful
template in Definition Objects and build abstraction for applications.

## Why CUE ? 

Why does KubeVela choose CUE? [Points of Cedric Charly](https://blog.cedriccharly.com/post/20191109-the-configuration-complexity-curse/) speaks well,
let me conclude here.

* **CUE is designed for large scale configuration.**
 CUE deliberately opts for the graph unification model used in computational linguistics instead of the traditional
 inheritance model. The graph model can help KubeVela to build a clear view of resources' relationships and dependency.
 For large scale infrastructure, that will be a complex, tightly interconnected graph of
 resources that describes an organization's entire computing environment. In this case, CUE has the ability to understand a
 configuration worked on by engineers across a whole company and to safely change a value that modifies thousands of
 objects in a configuration.
* **CUE supports first-class code generation and automation.**
 A design goal of CUE is to have code that is straightfoward for humans to write, but is also simple for machines to
 generate. This goal highly consistent with KubeVela which wants to offer abstractions to bridge the gap between concepts
 used by app developers and kubernetes. CUE can integrate with existing tools and workflows naturally while other tools
 would have to build complex custom solutions. CUE can generate Kubernetes definitions from Go code and OpenAPI schemas
 and immediately work with resources directly or build higher level libraries.
* **CUE integrates very well with Go.**
 KubeVela is built with GO just like most projects of the while Kubernetes system. CUE is also implemented in and
 exposes a rich client API in Go. KubeVela integrates with CUE as its core library and works as a Kubernetes CRD controller.
 With the help of CUE, KubeVela can easily handle data constraint problems.
 
If you want to go deeper I recommend reading [The Logic of CUE](https://cuelang.org/docs/concepts/logic/)
to understand the theoretical foundation and what makes CUE different from other configuration languages.

## CUE in KubeVela

Let's go back to discuss how does CUE be used in KubeVela. As you know, KubeVela helps platform builder to build abstraction
from Kubernetes resources to an application. We will use a [K8s Deployment](https://kubernetes.io/docs/concepts/workloads/controllers/deployment/)
as example to explain how was it implemented.

In KubeVela we usually build a WorkloadDefinition to generate K8s Deployment as it's workload-like resources.
A complete WorkloadDefinition example like below:

```yaml
apiVersion: core.oam.dev/v1alpha2
kind: WorkloadDefinition
metadata:
  name: mydeploy
spec:
  definitionRef:
    name: deployments.apps
  template: |
    parameter: {
        name: string
        image: string
    }
    output: {
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
                    }]
                }}}
    }
```

In this example, the `template` field is totally CUE, it contains two keywords, `output` and `parameter`.
The `output` defines what will be rendering out by the template. The `parameter` defines the input parameters which can
be part of the application. 

Let's try to write the CUE template step by step.

As you see, below is a Deployment YAML, most of the fields can be hidden and only expose `image` and `env` field for end user.

```yaml
apiVersion: apps/v1
kind: Deployment
meadata:
  name: mytest
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

The first step is to convert the YAML to JSON and put the whole json object into the `output` keyword field.
CUE is a superset of JSON: any valid JSON file is a valid CUE file. It provides some conveniences such as you can omit
some quotes from field names without special characters.

Here is the converted result:
                                                                                                            
```cue
output: {
    apiVersion: "apps/v1"
    kind:       "Deployment"
    metadata: name: "mytest"
    spec: {
        selector: matchLabels: {
            "app.oam.dev/component": "mytest"
        }
        template: {
            metadata: labels: {
                "app.oam.dev/component": "mytest"
            }
            spec: {
                containers: [{
                    name:  "mytest"
                    image: "nginx:v1"
                    env: [{name:"a",value:"b"}]
                }]
            }}}
}
```

Here are all conveniences add by CUE as a superset of JSON:

* C-style comments,
* quotes may be omitted from field names without special characters,
* commas at the end of fields are optional,
* comma after last element in list is allowed,
* outer curly braces are optional.

After that we add `parameter` keyword into the template, and use it as a variable reference, this is basic CUE grammar.

Fields of keyword `parameter` will be detected by KubeVela and be exposed to users using in application.

```cue
parameter: {
    name: string
    image: string
}
output: {
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
                }]
            }}}
}
```

Finally, you can put the whole CUE template into the `template` field of WorkloadDefinition object. That's all you need
to know to create a basic KubeVela capability.

## More Advanced Usage of CUE Grammar in Definition

In this section, we will introduce some more advanced CUE grammar to use in KubeVela.  

### Structural parameter

If you have some complex type of parameters in your template, and want to define a struct or embed struct as parameters,
then you could use structural parameter.

1. Define a struct type in, it includes a struct, a string and an integer.
    ```
    #Config: {
     name:  string
     value: int
     other: {
       key: string
       value: string
     }
    }
    ```

2. Use the struct defined in the `parameter` keyword, and use it as an array list.
    ```
    parameter: {
     name: string
     image: string
     configSingle: #Config
     config: [...#Config]
    }
    ```

3. In `output` keyword, it's referenced the same way with other normal field s.
    ```
    output: {
       ...
             spec: {
                 containers: [{
                     name:  parameter.name
                     image: parameter.image
                     env: parameter.config
                 }]
             }
        ...
    }
    ```

4. The structural field `config` can be easily used in Application like below:
    ```
    apiVersion: core.oam.dev/v1alpha2
    kind: Application
    metadata:
      name: website
    spec:
      components:
        - name: backend
          type: mydeploy
          settings:
            image: crccheck/hello-world
            name: mysvc
            config:
             - name: a
               value: 1
               other:
                 key: mykey
                 value: myvalue
    ```
    
### Conditional Parameter

Conditional parameter can be used to decide template condition logic. 
Below is an example that when `useENV=true`, it will render env section, otherwise, it will not.

```
parameter: {
    name:   string
    image:  string
    useENV: bool
}
output: {
    ...
    spec: {
        containers: [{
            name:  parameter.name
            image: parameter.image
            if parameter.useENV == true {
                env: [{name: "my-env", value: "my-value"}]
            }
        }]
    }
    ...
}
```

### Optional Parameter and Default Value

Optional parameter can be optional, that usually works with conditional logic. If some field does not exit, the CUE
grammar is `if _variable_ != _|_`, the example is like below:

```
parameter: {
    name: string
    image: string
    config?: [...#Config]
}
output: {
    ...
    spec: {
        containers: [{
            name:  parameter.name
            image: parameter.image
            if parameter.config != _|_ {
                config: parameter.config
            }
        }]
    }
    ...
}
```

Default Value is marked with a `*` prefix. It's used like 

```
parameter: {
    name: string
    image: *"nginx:v1" | string
    port: *80 | int
    number: *123.4 | float
}
output: {
    ...
    spec: {
        containers: [{
            name:  parameter.name
            image: parameter.image
        }]
    }
    ...
}
```

So if a parameter field is neither a parameter with default value nor a conditional field, it's a required value.

### Loop 

#### Loop for map type

```cue
parameter: {
    name:  string
    image: string
    env: [string]: string
}
output: {
    spec: {
        containers: [{
            name:  parameter.name
            image: parameter.image
            env: [
                for k, v in parameter.env {
                    name:  k
                    value: v
                },
            ]
        }]
    }
}
```

#### Loop for slice

```cue
parameter: {
    name:  string
    image: string
    env: [...{name:string,value:string}]
}
output: {
  ...
     spec: {
        containers: [{
            name:  parameter.name
            image: parameter.image
            env: [
                for _, v in parameter.env {
                    name:  v.name
                    value: v.value
                },
            ]
        }]
    }
}
```

### Import CUE internal packages

CUE has [lots of internal packages](https://pkg.go.dev/cuelang.org/go@v0.2.2/pkg) which also can be used in KubeVela.

Below is an example that use `strings.Join` to concat string list to one string. 

```cue
import ("strings")

parameter: {
	outputs: [{ip: "1.1.1.1", hostname: "xxx.com"}, {ip: "2.2.2.2", hostname: "yyy.com"}]
}
output: {
	spec: {
		if len(parameter.outputs) > 0 {
			_x: [ for _, v in parameter.outputs {
				"\(v.ip) \(v.hostname)"
			}]
			message: "Visiting URL: " + strings.Join(_x, "")
		}
	}
}
```














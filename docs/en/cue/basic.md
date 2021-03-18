# CUE Basic

This document will explain how to use [CUE](https://cuelang.org/) to encapsulate a given capability in Kubernetes and make it available to end users to consume in `Application` CRD. Please make sure you have already learned about `Application` custom resource before reading the following guide. 

## Overview

The reasons for KubeVela supports CUE as first class templating solution can be concluded as below:

- **CUE is designed for large scale configuration.** CUE has the ability to understand a
 configuration worked on by engineers across a whole company and to safely change a value that modifies thousands of objects in a configuration. This aligns very well with KubeVela's original goal to define and ship production level applications at web scale.
- **CUE supports first-class code generation and automation.** CUE can integrate with existing tools and workflows naturally while other tools would have to build complex custom solutions. For example, generate OpenAPI schemas wigh Go code. This is how KubeVela build developer tools and GUI interfaces based on the CUE templates.
- **CUE integrates very well with Go.**
 KubeVela is built with GO just like most projects in Kubernetes system. CUE is also implemented in and exposes a rich API in Go. KubeVela integrates with CUE as its core library and works as a Kubernetes controller. With the help of CUE, KubeVela can easily handle data constraint problems.

> Pleas also check [The Configuration Complexity Curse](https://blog.cedriccharly.com/post/20191109-the-configuration-complexity-curse/) and [The Logic of CUE](https://cuelang.org/docs/concepts/logic/) for more details.

## Parameter and Template

A very simple `WorkloadDefinition` is like below:

```yaml
apiVersion: core.oam.dev/v1alpha2
kind: WorkloadDefinition
metadata:
  name: mydeploy
spec:
  definitionRef:
    name: deployments.apps
  schematic:
    cue:
      template: |
        parameter: {
        	name:  string
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
        			}
        		}
        	}
        }
```

The `template` field in this definition is a CUE module, it defines two keywords for KubeVela to build the application abstraction:
- The `parameter` defines the input parameters from end user, i.e. the configurable fields in the abstraction.
- The `output` defines the template for the abstraction. 

## CUE Template Step by Step

Let's say as the platform team, we only want to allow end user configure `image` and `name` fields in the `Application` abstraction, and automatically generate all rest of the fields. How can we use CUE to achieve this?

We can start from the final resource we envision the platform will generate based on user inputs, for example:

```yaml
apiVersion: apps/v1
kind: Deployment
meadata:
  name: mytest # user inputs
spec:
  template:
    spec:
      containers:
      - name: mytest # user inputs
        env:
        - name: a
          value: b
        image: nginx:v1 # user inputs
    metadata:
      labels:
        app.oam.dev/component: mytest # generate by user inputs
  selector:
    matchLabels:
      app.oam.dev/component: mytest # generate by user inputs
```  

Then we can just convert this YAML to JSON and put the whole JSON object into the `output` keyword field:
                                                                                                            
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
            }
        }
    }
}
```

Since CUE as a superset of JSON, we can use:

* C style comments,
* quotes may be omitted from field names without special characters,
* commas at the end of fields are optional,
* comma after last element in list is allowed,
* outer curly braces are optional.

After that, we can then add `parameter` keyword, and use it as a variable reference, this is the very basic CUE feature for templating.

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
            }
        }
    }
}
```

Finally, you can put the above CUE module in the `template` field of `WorkloadDefinition` object and give it a name. Then end users can now author `Application` resource reference this definition as workload type and only have `name` and `image` as configurable parameters.

## Advanced CUE Templating

In this section, we will introduce advanced CUE templating features supports in KubeVela.  

### Structural Parameter

This is the most commonly used feature. It enables us to expose complex data structure for end users. For example, environment variable list.

A simple guide is as below:

1. Define a type in the CUE template, it includes a struct (`other`), a string and an integer.

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

2. In the `parameter` section, reference above type and define it as `[...#Config]`. Then it can accept inputs from end users as an array list.

    ```
    parameter: {
       name: string
       image: string
       configSingle: #Config
       config: [...#Config] # array list parameter
    }
    ```

3. In the `output` section, simply do templating as other parameters.

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

4. As long as you install a workload definition object (e.g. `mydeploy`) with above template in the system, a new field `config` will be available to use like below:
   
  ```yaml
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
          config: # a complex parameter
           - name: a
             value: 1
             other:
               key: mykey
               value: myvalue
  ```


### Conditional Parameter

Conditional parameter can be used to do `if..else` logic in template.

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

### Optional and Default Value

Optional parameter can be skipped, that usually works together with conditional logic. 

Specifically, if some field does not exit, the CUE grammar is `if _variable_ != _|_`, the example is like below:

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

#### Loop for Map

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

#### Loop for Slice

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

### Import CUE Internal Packages

CUE has many [internal packages](https://pkg.go.dev/cuelang.org/go@v0.2.2/pkg) which also can be used in KubeVela.

Below is an example that use `strings.Join` to `concat` string list to one string. 

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

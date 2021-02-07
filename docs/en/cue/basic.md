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
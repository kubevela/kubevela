# Appfile: Extensible, User-friendly Application Config Format

- Owner: Hongchao Deng (@hongchaodeng)
- Date: 10/14/2020
- Status: Implemented

## Table of Contents

- [Intro](#intro)
- [Goals](#goals)
- [Proposal](#proposal)
  - [Registration via Definition/Capability](#registration-via-definitioncapability)
  - [Templating](#templating)
  - [CLI/UI interoperability](#cliui-interoperability)
  - [vela up](#vela-up)
- [Examples](#examples)

## Intro

Vela supports a user-friendly `docker-compose` style config format called `Appfile`. It allows you to define an application's workloads and traits with an opinionated, simplified API interface.

Here's an example to deploy a NodeJS express service:

```yaml
services:
  express-server:
    # this image will be used in both build and deploy config
    image: oamdev/testapp:v1

    build:
      # Here more runtime specific build templates will be supported, like NodeJS, Go, Python, Ruby.
      docker:
        file: Dockerfile
        context: .

    cmd: ["node", "server.js"]

    route:
      domain: example.com
      http: # match the longest prefix
        "/": 8080

    env:
      - FOO=bar
      - FOO2=sec:my-secret # map the key same as the env name (`FOO2`) from my-secret to env var
      - FOO3=sec:my-secret:key # map specific key from my-secret to env var
      - sec:my-secret # map all KV pairs from my-secret to env var

    files: # Mount secret as a file
      - /mnt/path=sec:my-secret

    scale:
      replicas: 2
      auto: # automatic scale up and down based on given metrics
        range: "1-10"
        cpu: 80 # if cpu utilization is above 80%, scale up
        qps: 1000 # if qps is higher than 1k, scale up

    canary: # Auto-create canary deployment. Only upgrade after verify successfully.
      replicas: 1 # canary deployment size
      headers:
        - "foo:bar.*"
```

Save this file to project root dir, and run:

```bash
vela up
```

It will build container image, render deployment manifests in yaml, and apply them to the server.

### Extensible Design

The Appfile could be extended with more configurations by adding more capabilities to the OAM system. The config fields in Appfile are strongly correlated to the [capabilities system of OAM](../../docs/en/design.md#capability-oriented) – Config fields are registered in the capabilities system and exposed via a [CUE template](https://cuelang.org/).

Here is an example of a capability definition that platform builders register:

```yaml
apiVersion: core.oam.dev/v1alpha2
kind: WorkloadDefinition
metadata:
  name: webservice
spec:
  definitionRef:
    name: deployments.apps
  extension:
    template: |
      parameter: {
        // +vela:cli:enabled=true
        // +vela:cli:usage=specify commands to run in container
        // +vela:cli:short=c
        cmd: [...string]
      
        env: [...string]
      
        files: [...string]

        image: string
      }
      
      output: {
        apiVersion: "apps/v1"
        kind: "Deployment"
        metadata:
          name: context.name
        spec: {
          selector: {
            matchLabels:
              app: context.name
          }
          template: {
            metadata:
              labels:
                app: context.name
            spec: {
              containers: [{
                name:  context.name
                image: parameter.image
                command: parameter.cmd
              }]
            }
          }
        }
      }
```

Apply the file to APIServer, and the fields will be extended into Appfile. Note that there are some conventions that differs Workloads and Traits, and around CLI flags. We will cover that more detailedly below.

## Goals

The Appfile design has the following goals:

1. Provide a user friendly, `docker-compose` style config format to developers.
2. Configuration fields can be extended by registering more capabilities into OAM runtime.

In the following, we will discuss technical details of the proposed design.

## Proposal

### Registration via Definition/Capability

Vela allows platform builders to extend Appfile config fields by registering them via [capabilities system of OAM](../../docs/en/design.md#capability-oriented).

The entire template should be put under `spec.extension.template` as raw string:

```yaml
apiVersion: core.oam.dev/v1alpha2
kind: WorkloadDefinition | TraitDefinition
...
spec:
  extension:
    template: |
      parameter: {
      ...
```

By running `vela system update` or other similar commands, vela cli will read all definitions from APIServer and sink necessary information locally including templates. The templates will be further used to render final deploy manifests.

### Templating

Vela allows platform builders to write bespoke templates to extend Appfile configs. 

#### Exposing Parameters

A template starts with `parameter` and its definition:

```yaml
parameter: {
  // +vela:cli:enabled=true
  // +vela:cli:usage=specify commands to run in container
  // +vela:cli:short=c
  cmd: [...string]
}
```

Here is the takeout:
* The `parameter` defines the user input fields and is used to render final output with user input values. These fields will be exposed to users in Appfile.

Note that there is difference in how Workload and Trait expose parameters.

For Workload, each service will have a reserved field called `type` which is *webservice* by default. 
Then all parameters are exposed as first level field under the service.

```yaml
services:
  express-server:
    # type: webservice (default) | task
    cmd: ["node", "server.js"]
```

For Trait, its type will be used as the name to contain its parameters. There is a restriction that the trait type should not conflict any of the Workload parameters' first level name.

```yaml
services:
  express-server:
    route: # trait type
      domain: example.com
      http: # match the longest prefix
        "/": 8080
    
    # Workload parameters. The first level names do not conflict with trait type.
    cmd: ... 
    env: ...
```


#### Rendering Outputs

A template should also have an `output` block:

```yaml
output: {
  apiVersion: "apps/v1"
  kind: "Deployment"
  metadata:
    name: context.name
  spec: {
        ...
        containers: [{
          name:  context.name
          image: parameter.image
          command: parameter.cmd
        }]
  }
}
```

Here is the takeout:
* The object defined within `output` block will be the final manifest which is to `kubectl apply`.
* `parameter` is used here to render user config values in.
* A new object called `context` is used to render output. This is defined within vela-cli and vela-cli will fill its values based on each service dynamically. In above example, here is the value of the `context`:
    ```yaml
    context:
      name: express-server
    ```
    You can check the definition of `context` block via `vela template context`.

Note that a TraitDefinition can have multiple outputs. In such case, just dismiss the `output` block and provide `outputs` block:

```yaml
outputs: service: {
        ...
}

outputs: ingress: {
        ...
}
```

Under the hood, vela-cli will iterate over all services and generate one AppConfig to contain them, and for each service generate one Component and multiple traits.

### CLI/UI Interoperability

For UI, The definition in a template will be used to generate v3 OpenAPI Schema and the UI will use that to render forms.

For CLI, a one level parameter can be exposed via CLI by adding the following "tags" in the comment:

```yaml
parameter: #webservice
#webservice: {
  // +vela:cli:enabled=true
  // +vela:cli:usage=specify commands to run in container
  // +vela:cli:short=c
  cmd: [...string]
  ...
}
```

Here is the takeout:
- The name of the parameter will be added as a flag, i.e. `--cmd`
- "enabled" indicates whether this parameter should be exposed
- "usage" is shown in help info
- "short" is the short flag, i.e. `-c`

### `vela up`

The vela-cli will have an `up` command to provide seamless workflow experience. Provide an `vela.yaml` Appfile in the same directory that you will run `vela up` and it is good to go. There is an example under `examples/testapp/` .

## Examples

## Multiple Services

```yaml
services:
  frontend:
    build:
      image: oamdev/frontend:v1
      docker:
        file: ./frontend/Dockerfile
        context: ./frontend
    cmd: ["node", "server.js"]

  backend:
    build:
      image: oamdev/backend:v1
      docker:
        file: ./backend/Dockerfile
        context: ./backend
    cmd: ["node", "server.js"]
```

### Multiple Outputs in TraitDefinition

```yaml
apiVersion: core.oam.dev/v1alpha2
kind: TraitDefinition
metadata:
  name: route
spec:
  definitionRef:
    name: routes.standard.oam.dev
  extension:
    template: |
      parameter: #route
      #route: {
        domain: string
        http: [string]: int
      }
      
      // trait template can have multiple outputs and they are all traits
      outputs: service: {
        apiVersion: "v1"
        kind: "Service"
        metadata:
          name: context.name
        spec: {
          selector:
            app: context.name
          ports: [
            for k, v in parameter.http {
              port: v
              targetPort: v
            }
          ]
        }
      }
      
      outputs: ingress: {
        apiVersion: "networking.k8s.io/v1beta1"
        kind: "Ingress"
        spec: {
          rules: [{
            host: parameter.domain
            http: {
              paths: [
                for k, v in parameter.http {
                  path: k
                  backend: {
                    serviceName: context.name
                    servicePort: v
                  }
                }
              ]
            }
          }]
        }
      }
```

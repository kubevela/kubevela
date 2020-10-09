# Appfile: Extensible, User-friendly API Config Format

- Owner: Hongchao Deng (@hongchaodeng)
- Date: 10/13/2020
- Status: Implemented

## Table of Contents

- [Intro](#intro)
- [Goals](#goals)
- [Proposal](#proposal)
- [Examples](#examples)

## Intro

Vela supports a developer-friendly `docker-compose` style config format called `Appfile`. It allows you to define an application's workloads and traits with an opinionated, simplified API interface.

Here's an example to deploy a NodeJS express service:

```yaml
services:
  express-server:
    build:
      image: oamdev/testapp:v1
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
      replica: 2
      auto: # automatic scale up and down based on given metrics
        range: "1-10"
        cpu: 80 # if cpu utilization is above 80%, scale up
        qps: 1000 # if qps is higher than 1k, scale up

    canary: # Auto-create canary deployment. Only upgrade after verify successfully.
      replica: 1 # canary deployment size
      headers:
        - "foo:bar.*"
```

Save this file to project root dir, and run:

```bash
vela up
```

It will build container image, render deployment manifests in yaml, and apply them to the server.

### Extensible Design

The Appfile could be extended with more configurations by adding more capabilities to the OAM system. The config fields in Appfile are strongly correlated to the [capabilities system of OAM](https://github.com/oam-dev/kubevela/blob/master/DESIGN.md#capability-register-and-discovery) – they are registered in the capabilities system and exposed in a simplified format via a [CUE template](https://cuelang.org/).

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
      parameter: #webservice
      #webservice: {
        // +vela:cli:enabled=true
        // +vela:cli:usage=specify commands to run in container
        // +vela:cli:short=c
        cmd: [...string]
      
        env: [...string]
      
        files: [...string]
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
                image: context.image
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

In the following, we will go through the proposal with more details on technical design.

## Proposal

### 

## Examples

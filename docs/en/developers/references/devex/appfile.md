# Appfile

## Description

Appfile is the single source-of-truth of the application description in KubeVela. It captures the application architecture and behaviors of its components following Open Application Model.

Before learning about Appfile's detailed schema, we recommend you to get familiar with core [concepts and glossaries](../../../concepts.md) in KubeVela.

## Schema

List of all available sections for Appfile.

```yaml
name: _app-name_

services:
  _service-name_:
    # If `build` section exists, this field will be used as the name to build image. Otherwise, KubeVela will try to pull the image with given name directly.
    image: oamdev/testapp:v1

    build:
      docker:
        file: _Dockerfile_path_ # relative path is supported, e.g. "./Dockerfile"
        context: _build_context_path_ # relative path is supported, e.g. "."

      push:
        local: kind # optionally push to local KinD cluster instead of remote registry

    type: webservice (default) | worker | task

    # detailed configurations of workload
    ... properties of the specified workload  ...

    _trait_1_:
      # properties of trait 1

    _trait_2_:
      # properties of trait 2

    ... more traits and their properties ...
  
  _another_service_name_: # more services can be defined
    ...
  
```
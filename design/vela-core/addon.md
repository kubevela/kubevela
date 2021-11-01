# AddonDefinition: Managing Addons via Declarative APIs

## Background

Currently the addon management system is glueing a couple of inconsistent concepts:

- Using `Application` as the controller to drive installation process
- Using go template to render parameters
- Need more information to describe the addon as discussed in [this issue](https://github.com/oam-dev/catalog/pull/115)

This has introduced confusion to users about addons.
We need a better concept to describe what an addon is and simplify addon management.


## Proposal

This proposal tries to solve the following problems:

- We need to provide common information about an addon such as version, tags, definitions.
- We need to unify templating methods using CUE. We shouldn't expose go template parameters.
- We need to provide more flexibility to describe addon installation process.

To solve this, we propose to add an AddonDefinition.
Here is an example:

```yaml
kind: AddonDefinition
metadata:
  name: fluxcd
  namespace: vela-system
spec:
  version: 1.2.3
  description: Extended workload to do continuous and progressive delivery

  # contact:
  #   author: xxx 
  #   email: xxx@xxx.com

  deployTo:
    control_plane: true
    runtime_cluster: false
  
  tags:
  - extended_workload
  - gitops

  definitions:
  - name: helm
    type: component
    description: >
      helps to deploy a helm chart from everywhere: git repo, helm repo, or S3 compatible bucket.
    url: https://kubevela.io/docs/end-user/components/helm

  - name: kustomize
    type: component
    description: >
      helps to deploy a kustomize style artifact from a git repo.
    url: https://kubevela.io/docs/end-user/components/kustomize

  dependencies:
  - namespace: addon_namespace
    name: addon_name

  # Here defines the addon installation stepps and parameterization in CUE.
  # This would provide flexibility and consistency for the addon management system within KubeVela.
  template: |
    apply: op.#Apply {
      value: {
        kind: Application
        ...
      }
    }
    parameters: {
      accessKey: string
      secretKey: string
    }
```

Under the hood, the controller will:

- run the steps to install the addon
- provide json schema of exposed parameters

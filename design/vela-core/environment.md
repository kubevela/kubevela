# Managing Environments As Shared Bases For Application Deployment

## Background

A team of application development usually needs to prepare some shared environments for developers to deploy applications.
For example, most teams will have two environments, a *dev* environment for testing their applications in live instances,
and a *prod* environment for deploying applications to serve live requests. The environment is a logical group bundling common resources that multiple application deployments might depend on.

Here are some examples of what users want to define in an environment:

- K8s Cluster. *dev* environment might have smaller clusters while *prod* might use large and complex clusters.
- System components and definitions. *dev* environment might install different Operators & CRDs from *prod* environment, e.g. *dev* might choose local logging while *prod* might use ELK logging.
- Network boundary. *dev* and *prod* environments would have different VPC setups, gateway endpoints, and firewall settings.
- Secret and config. *dev* might use locally-run MySQL, while *prod* might use managed RDS. The DB connection credentials would be saved differently.
- Policies. This is more important in *prod* environment that it would usually have more restrictions on application deployments, like security scanning, config checks, SLO checks, etc.

All of above examples and more are bundled as a shared base for applications to run on.
This is usualy done by an admin for the entire application team.
By adding Environment API, Vela aligns with user workflow better,
provides consistent view and pluggable options for end users,
and empowering platform builders to add modular environment capabilities. In the following, we will propose the design of API and give an implementation plan.


## Proposal

### 1. Environment CRD

We propose to add an Environment CRD to describe the shared resources for applications to base on.

```yaml
kind: Environment
metadata:
  name: prod
spec:
  # A list of pluggable EnvConfig resources.
  # The resources will be parsed and applied when Environment CR is appled.
  configs:
    - type: cluster
      properties:
        name: prod-cluster
        selector:
          tier: prod

    - type: system-setup
      properties:
        components:
          - type: helm-git
            properties:
              git: git-repo-url
              path: path/to/chart
              valuePath: value.yaml
          - type: kustomize-git
            properties:
              kube:
                git: git-repo-url
                path: path/to/dir
    
    - type: secrets
      properties:
        redis:
          url: redis-url
          password: redis-password
    
    - type: policies
      properties:
        quota: ...
        security: ...
        slo: ...
```

We also need to add an optional field `environment` in Application:

```yaml
kind: Application
spec:
  # If set, app controller will add the AppRevision into the Environment
  # CR spec after generating AppRevision before executing Workflow.
  environment: prod
```

### 2. EnvConfigDefinition CRD

An Environment can have multiple EnvConfigs. The schema is defined in EnvConfigDefinition CRD:

```yaml
apiVersion: core.oam.dev/v1beta1
kind: EnvConfigDefinition
metadata:
  name: system-setup
spec:
  # If this is true, app controller will hold the app workflow execution
  # and wait for EnvConfig resource to be ready.
  # Some EnvConfig will need to setup first before deploying applications,
  # e.g. setting up VPC and firewall rules, preparing DB connection secret.
  waitPerApp: true

  schematic:
    cue:
      template: |
        output: {
          ...
        }
        parameters: {
          ...
        }
```

## Technical Details

With the introduction of Environment, we expect the user workflow will become:

1. An admin (or similar role) will setup the Environments first.
1. Developers will deploy Applications to specified Environments.

For (1), we will add an Environment controller that will recocnile Environment CRs and render and emit EnvConfig resources.

For (2), the app controller will be modified to provide interaction between Application and Environment. After generating AppRevision, if environment is set, it will json-marshal the following object and add it to the spec of specified environment CR:

```yaml
kind: Environment
metadata:
  name: prod # This is the name specified in spec.environment of Application
spec:
  # This is the list of app revisions to be deployed to this env.
  # This list is updated by App Controller during reconcile
  appRevisions:
    - name: my-app-v1
    - name: another-app-v1
```

Note that EnvConfigDefinition has a field called `waitPerApp`. If it is true, app controller will wait until the AppRevision name in `status.readyAppRevisions`:

```yaml
kind: Environment
metadata:
  name: prod
status:
  # This is updated by EnvConfig controller if needed.
  readyAppRevisions:
    - name: my-app-v1
```

## Considerations


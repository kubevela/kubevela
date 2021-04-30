# Environment-Based Application Configuration Management

## Background

KubeVela users want an "Environment" concept to define shared-base for application deployments. For example, different applications might share the same cluster, DB connection secrets, capability definitions, network boundaries (VPC), etc. An environment encapsulates these shared resources into one concept and enables better manageability.

Here are some user scenarios from our offline survey:

- Users have a template base, and want to make modifications based on selected environment. For example, set different image names for dev/prod environments.
- Users want the definitions in dev/prod environments to be different. For example, the logging trait might be uploaded to OSS/S3 buckets in dev or sinked to ELK in prod.
- Users want the secrets in dev/prod environments to be different. For example, the DB connection secrets are different in dev/prod environments.


## Proposal

This design proposes to add the following CRDs to satisfy user requirements.

### 1 Environment CRD

Environment CRD defines the shared-base environment to deploy applications to.

```yaml
kind: Environment
spec:
  clusters: # Apply to the following clusters.
    - prod-cluster

  secrets: # The secrets that will be created/updated.
    - name: redis
      data:
        url: redis-url
        password: redis-password

  definitions: # The definitions that will be created/updated.
    - type: Component
      name: function
      source:
        kube: # using kubectl apply 
          git: git-repo-url
          path: path/to/definition # path to a file or a directory
    - type: Trait
      name: logging
      source:
        helm: # using helm install/upgrade
          git: git-repo-url
          path: path/to/chart
          valuePath: value.yaml
```

We would add a new env-controller to handle this CR. When a new Environment CR is applied, the env-controller will apply the secrets and definitions to the corresponding clusters. Note that the cluster objects are defined in [v1beta1/cluster_types.go](https://github.com/oam-dev/kubevela/blob/675b0e24db0cedde11c9870242763957b0012f99/apis/core.oam.dev/v1beta1/cluster_types.go#L45-L52).

### 2. ApplicationTemplate CRD

The ApplicationTemplate CRD defines the base template shared to further parameterize or patch more config to.

```yaml
kind: ApplicationTemplate
spec:
  # base template written in CUE
  template: |
    output: {
      apiVersion: core.oam.dev/v1alpha2
      kind: Application
      metadata:
        name: context.name
      spec:
        components:
          - name: instance
            type: webservice
            properties:
              image: test-image
              cmd:
                - sleep 100
            traits:
              - name: ingress
          - name: database
            type: parameter.databaseType
        environment:
          namespace: parameter.project
    }
    parameter: {
      project: string | *""
      databaseType: string | *"rds"
    }
```

Note that this is a data object to reference to. It wouldn't have any effects if applied by itself.

### 3. ApplicationSetting CRD

With the above ApplicationTemplate, how could users pick a template and define environment-specific parameters? This is what ApplicationSetting CRD defines.

```yaml
kind: ApplicationSetting
spec:
  applications:
  - targetEnvs: # names of the Environments to deploy to
      - dev
    template: my-app-tpl # name of the ApplicationTemplate
    config:
      parameters:
        project: dev
        databaseType: mysql
  - targetEnvs:
      - prod
    template: my-app-tpl # name of the ApplicationTemplate
    config:
      parameters:
        project: prod
        databaseType: rds
      patch: # kustomize-style overlay patch to base template
        spec:
          components:
            - name: instance
              properties:
                image: prod-image
```


We would add a new appsetting-controller to handle this CR. When a new ApplicationSetting CR is applied, the appsetting-controller will render the final Application based on the template and the parameters based on the environment, and deploy to targeted clusters defined in the environments.

### Summary: User workflow

With the above concepts, the user workflow goes as:

- The ops team setup Environments first.
- The ops team prepare ApplicationTemplates that will be reused.
- The developers write ApplicationSettings, including choosing the templates, the  environment and providing application-concerned parameters:
  - KubeVela will render the final Application based on the template and the parameters based on the environment.
  - KubeVela will prepare the secrets, definitions, etc. defined in the environment. Applications can use the definitions, secrets in the environment.

## Considerations

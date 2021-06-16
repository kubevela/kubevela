# Managing Environments As Shared Bases For Application Deployment

## Background

A team of application development usually needs to prepare some shared environments for developers to deploy applications to.
For example, most teams will have two environments, a *dev* environment for testing applications,
and a *prod* environment for running applications to serve live traffic.
The environment is a logical concept grouping common resources that multiple application deployments would depend on.

A list of resources we want to define per environment:

- K8s Clusters. Different environments might have clusters of different size and version, e.g.
  small v1.10 k8s cluster for *dev* environment and large v1.9 k8s cluster for *prod* environment.
- Admin policies. Production environment would usually set global policies on all application deployments,
  like chaos testing, SLO requirements, security scanning, misconfiguration detection.
- Operators & CRDs. Environments contains a variety of Operators & CRDs as system capabilities.
  These operators can include domain registration, routing, logging, monitoring, autoscaling, etc.
- Shared services. Environments contains a variety of system services that are be shared by applications.
  These resources can include DB, cache, load balancers, API Gateways, etc.

With the introduction of environment, the user workflow on Vela works like:

- Platform team defines system Components for Environments.
- Platform team deploys Environments using system Components.
- Platform team defines user Components for Applications.
- Developers choose user Components and Environments to deploy Applicatons.

When deploying environments, there are additional requirements:

- The ability to describe dependency relations between environments. For example, both *dev* and *prod* environments
  would depend on a common environment that sets up operators both *dev* and *prod*  would use.


## Environment CRD

We propose to add an Environment CRD:

```yaml
kind: Environment
metadata:
  name: prod
spec:
  # An environment consists of multiple system resources.
  # An environment is a grouping of these shared resources for user applications.
  resources:
    - # mustOwn indicates that this resource should be owned by this env.
      mustOwn: true

      # template defines the resource template that will be applied.
      template:
        # We can use Application to deploy the resources that an environment needs.
        # An environment setup can be done just like deploying applications in system level.
        apiVersion: core.oam.dev/v1beta1
        kind: Application
        spec:
          ...

  # dependsOn indicates the other environments that this env depends on.
  # The environment will not apply its components until all dependencies exist.
  dependsOn:
    - ref:
        apiVersion: core.oam.dev/v1beta1
        kind: Environment
        name: common-operators
```

By adding this environment CRD, users can setup environments by deploying system applications --
the abstractions is flexible enough to do work like provisiniong cloud resources, handling db migration,
installing system operators, etc.
Additionally, environment dependency enables separating common setup work like setting up namespaces,
RBAC rules, accounts into reusable environment modules.


## Environment-Related Operators

The Environment CRD groups env-related resources together.
To satisfy the each resource and its use case, we also need to implement the supporting operators and definitions.
In the following, we will walk through each use case and discuss the design of solution.


### Case 1: K8s Cluster

In this case, an environment contains some clusters, and developers specify an environment to deploy apps.

Platform team would deploy an Environment:

```yaml
kind: Environment
metadata:
  name: prod-env
spec:
  resources:
    - template:
        apiVersion: core.oam.dev/v1beta1
        kind: Application
        metadata:
          name: prod-env-clusters
        spec:
          components:
            - name: prod-env-clusters
              type: cluster-selector
              properties:
                # cluster with given names or matched labels will belong to this env
                clusterSelector:
                  names: ["prod-1"， "prod-2"]
                  labels:
                    tier: prod
```

Developers would define how to deploy to clusters based on environments via Workflow:

```yaml
kind: Application
metadata:
  name: my-app
spec:
  workflow:
    - name: deploy2clusters
      type: deploy2clusters
      properties:
        # This app will be deployed to the clusters of specified env.
        env: prod-env

        # propagation indicates how to propagate the app to the clusters in this environment.
        #   all: all clusters will get one instance of the app
        #   twin: pick two clusters to deploy two instances of the app
        #   single: pick one cluster to deploy only one instance of the app
        propagation: all
```

Under the hood, the `cluster-selector` Components would be translated to data objects (e.g. ConfigMap
or dedicated Cluster objects) that contains cluster information.
The `deploy2clusters` Workflow Steps would trigger supporting Operator to read cluster info
from specified env and handles progapation of the app's resources to selected clusters.


### Case 2: Admin Policy

In this case, an environment contains some admin policies, and developers specify an environment to deploy apps.

Platform team would define an admin policy as a Component, and deploy such an environment as an Application:

```yaml
kind: Environment
metadata:
  name: prod-env
spec:
  resources:
    - template:
        apiVersion: core.oam.dev/v1beta1
        kind: Application
        metadata:
          name: prod-env-policies
        spec:
          components:
            - name: prod-env-slo-policy
              type: slo-policy
              properties:
                # slo indicates the service in this env must be 99.99% up
                slo: "99.99"
            - name: prod-env-chaos-policy
              type: chaos-policy
              properties:
                io:
                  action: fault
                  volumePath: /var/run/etcd
                  path: /var/run/etcd/**/*
                  percent: 50
                  duration: "400s"
```

Developers would specify an environment to deploy via Workflow, which automatically does policy checks in the background:

```yaml
kind: Application
metadata:
  name: my-app
spec:
  workflow:
    - name: check-policy-by-env
      type: check-policy-by-env
      properties:
        # This app will be checked against the policies in given env.
        env: prod-env
        # If policy checks failed, send alerts to spcified channels
        notifications:
          slack: slack_url
          dingding: dingding_url
```

Under the hood, the `slo-policy` and `chaos-policy` Components would be translated to data objects (e.g. ConfigMap
or dedicated Policy objects) that contains policy specification.
The `check-policy-by-env` Workflow Steps would trigger supporting Operator to register the apps
to given env's policy checks, and handle notifications.


### case 3: Operator & CRD

In this case, an environment defines the system Operators ands CRDs to be installed on a cluster.
Deploying such an environment will setup these services and definitions on the host cluster.

Platform team would define these Operators and CRDs as Components, and deploy such an environment as an Application:

```yaml
kind: Environment
metadata:
  name: prod-env
spec:
  resources:
    - template:
        apiVersion: core.oam.dev/v1beta1
        kind: Application
        metadata:
          name: prod-env-operators
        spec:
          components:
            - name: prod-monitoring-operator
              # Reuse the built-in Helm/Kustomize ComponentDefinition to deploy Operators from Git repo.
              type: helm-git
              properties:
                chart:
                  repository: operator-git-repo-url
                  name: monitoring-operator
                  version: 3.2.0
            - name: prod-logging-operator
              type: kustomize-git
              properties:
                sourceRef:
                  kind: GitRepository
                  name: logging-operator
                path: ./apps/prod
```

Under the hood, Vela will support built-in ComponentDefinitions to deploy Helm charts or Kustomize folders
via git url. We can use this functionality to deploy system operators.


### case 4: Shared Service

In this case, an environment defines shared services to be installed on the cluster.
Deploying such an environment will setup these services on the host cluster.

Platform team would define the shared services as Components, and deploy such an environment as an Application:

```yaml
kind: Environment
metadata:
  name: prod-env
spec:
  resources:
    - template:
        apiVersion: core.oam.dev/v1beta1
        kind: Application
        metadata:
          name: prod-env
        spec:
          components:
            - name: prod-policy-env
              # Reuse the built-in Helm ComponentDefinition to deploy Crossplane resource.
              type: helm-git
              properties:
                chart:
                  repository: crossplane-mysql-url
                  name: mysql
                  version: 3.2.0
            - name: prod-policy-env
              # Reuse the built-in Terraform ComponentDefinition to deploy cloud resource.
              type: terraform-git
              properties:
                module:
                  source: "git::https://github.com/catalog/kafka.git"
                outputs:
                  - key: queue_urls
                    moduleOutputName: queue_urls
                  - key: json_string
                    moduleOutputName: json_string
                variables:
                  - key: ACCESS_KEY_ID
                    sensitive: true
                    environmentVariable: true
                  - key: SECRET_ACCESS_KEY
                    sensitive: true
                    environmentVariable: true
                  - key: CONFIRM_DESTROY
                    value: "1"
                    sensitive: false
                    environmentVariable: true
```


## Implementation Plan

TODO: Here are the detailed planning of how to implement each support operator.


## Considerations

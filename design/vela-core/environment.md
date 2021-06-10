# Managing Environments As Shared Bases For Application Deployment

## Background

A team of application development usually needs to prepare some shared environments for developers to deploy applications to.
For example, most teams will have two environments, a *dev* environment for testing applications,
and a *prod* environment for running applications to serve live traffic.
The environment is a logical concept grouping common resources that multiple application deployments could depend on.

A list of resources we want to define per environment:

- K8s Clusters. Different environments might have clusters of different size and version, e.g. small v1.10 k8s cluster for *dev* environment and large v1.9 k8s cluster for *prod* environment.
- Admin policies. Production environment would usually set global policies on all application deployments,
  like chaos testing, SLO requirements, security scanning, misconfiguration detection.
- Operators & CRDs. Environments contains a variety of Operators & CRDs as system capabilities.
  These operators can include domain registration, routing, logging, monitoring, autoscaling, etc.
- Shared services. Environments contains a variety of system services that are be shared by applications.
  These resources can include DB, cache, load balancers, API Gateways, etc.

With the introduction of environment, the user workflow on Vela works like:

- Platform team defines system Definitions for Environments.
- Platform team deploys Environments.
- Platform team defines user Definitions for Application deployment.
- Developers choose user Definitions and Environments to deploy Applicatons.


## Proposal

Instead of adding a new API, we will propose how to implement environment concept via existing APIs (Application, Component, Workflow).
In the following, we will walk through each use case of environment concept and discuss the implementation of supporting Operators.

### Case 1: K8s Cluster

In this case, an environment contains some clusters, and developers specify an environment to deploy apps.

Platform team would define cluster info as a Component, and deploy such an environment as an Application:

```yaml
kind: Application
metadata:
  name: prod-env
spec:
  components:
    - name: prod-cluster-env
      type: cluster-env
      properties:
        # cluster with given names or matched labels will belong to this env (i.e. prod).
        clusterSelector:
          names: ["prod-1"， "prod-2"]
          labels:
            tier: prod
```

Developers would define how to deploy to cluters based on environments via Workflow:

```yaml
kind: Application
metadata:
  name: my-app
spec:
  workflow:
    - name: prod-cluster-deploy
      type: cluster-env-selector
      properties:
        # This app will be deployed to the clusters of given env.
        # This provides isolation between different environments.
        env: prod-env
        # propagation indicates how to propagate the app to the clusters in this environment.
        #   all: all clusters will get one instance of the app
        #   twin: pick two clusters to deploy two instances of the app
        #   single: pick one cluster to deploy only one instance of the app
        propagation: all
```

Under the hood, the `cluster-env` Components would be translated to data objects (e.g. ConfigMap
or dedicated Cluster objects) that contains cluster information.
The `cluster-env-selector` Workflow Steps would trigger supporting Operator to read cluster info
from specified env and handles progapation of the app's resources to selected clusters.


### Case 2: Admin Policy

In this case, an environment contains some admin policies, and developers specify an environment to deploy apps.

Platform team would define an admin policy as a Component, and deploy such an environment as an Application:

```yaml
kind: Application
metadata:
  name: prod-env
spec:
  components:
    - name: prod-policy-env
      type: policy-env
      properties:
        # slo indicates the service in this env must be 99.99% up
        slo: "99.99"
        # chaos indicates the service must be able to tolerate some chaos testing in this env
        chaos: true
```

Developers would specify an environment to deploy via Workflow, which automatically does policy checks in the background:

```yaml
kind: Application
metadata:
  name: my-app
spec:
  workflow:
    - name: prod-policy-env
      type: policy-env-selector
      properties:
        # This app will be checked against the policies in given env.
        env: prod-env
        # If policy checks failed, send alerts to spcified channels
        notifications:
          slack: slack_url
          dingding: dingding_url
```

Under the hood, the `policy-env` Components would be translated to data objects (e.g. ConfigMap
or dedicated Policy objects) that contains policy specification.
The `policy-env-selector` Workflow Steps would trigger supporting Operator to register the apps
to given env's policy checks, and handle other config such as notifications.


### case 3: Operator & CRD

In this case, an environment defines the system Operators ands CRDs to be installed on a cluster.
Deploying such an environment will setup these services and definitions on the host cluster.

Platform team would define these Operators and CRDs as Components, and deploy such an environment as an Application:

```yaml
kind: Application
metadata:
  name: prod-env
spec:
  components:
    - name: prod-policy-env
      # Reuse the built-in Helm/Kustomize ComponentDefinition to deploy Operators from Git repo.
      type: helm-git
      properties:
        chart:
          repository: operator-git-repo-url
          name: some-operator
          version: 3.2.0
    - name: prod-policy-env
      type: kustomize-git
      properties:
        sourceRef:
          kind: GitRepository
          name: some-operator
        path: ./apps/staging
```

Under the hood, Vela will support built-in ComponentDefinitions to deploy Helm charts or Kustomize folders
via git url. We can use this functionality to deploy system operators.


### case 4: Shared Service

In this case, an environment defines shared services to be installed on the cluster.
Deploying such an environment will setup these services on the host cluster.

Platform team would define the shared services as Components, and deploy such an environment as an Application:

```yaml
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

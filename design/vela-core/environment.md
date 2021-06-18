# Initilizing Environments For Application Deployment

## Background

A team of application development usually needs to initialize some shared environments for developers to deploy applications to.
For example, a team would initialize two environments, a *dev* environment for testing applications,
and a *prod* environment for running applications to serve live traffic.
The environment is a logical concept grouping common resources that multiple application deployments would depend on.

A list of resources we want to initialize per environment:

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
- Platform team initializes Environments using system Components.
- Platform team defines user Components for Applications.
- Developers choose user Components and Environments to deploy Applicatons.

When initializing environments, there are additional requirements:

- The ability to describe dependency relations between environments. For example, both *dev* and *prod* environments
  would depend on a common environment that sets up operators both *dev* and *prod* would use.


## Initialzer CRD

We propose to add an Initializer CRD:

```yaml
kind: Initializer
metadata:
  name: prod-env
spec:
  # components indicates the system components to initialize for an environment.
  components:
    - # mustOwn indicates that this resource should be owned by this env.
      mustOwn: true

      # It would use the same rendering function as in Application
      # to render and apply the final resource from Component Definition.
      name: prod-env-placement
      type: placement
      properties:
        clusterSelector:
          names: ["prod-1", "prod-2"]


  # dependsOn indicates the other initializers that this depends on.
  # The initializer will not apply its components until all dependencies exist.
  dependsOn:
    - ref:
        apiVersion: core.oam.dev/v1beta1
        kind: Initializer
        name: common-operators
```

By adding this initializer CRD, users can initializer environments by deploying system components --
the abstractions is flexible enough to do work like provisiniong cloud resources, handling db migration,
installing system operators, etc.
Additionally, initializer dependency enables separating common setup tasks like setting up namespaces,
RBAC rules, accounts into reusable modules.


## Initializer-Related Operators

The Initializer CRD groups initialization resources together.
To satisfy each initialization use case, we also need to implement the supporting operators and definitions.
In the following, we will walk through each use case and discuss the design of solution.


### Case 1: K8s Cluster

In this case, an initializer contains some clusters, and developers specify an initializer name to deploy apps.

Platform team would deploy an Initializer:

```yaml
kind: Initializer
metadata:
  name: prod-env
spec:
  components:
    - name: prod-env-placement
      type: placement
      properties:
        # clusters of specified names or matched labels will be selected
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
        # target indicates the name of the initializer
        # which contains the selected clusters information.
        target: prod-env

        # propagation indicates how to propagate the app to the clusters.
        #   all: all clusters will get one instance of the app
        #   twin: pick two clusters to deploy two instances of the app
        #   single: pick one cluster to deploy only one instance of the app
        propagation: all
```

Under the hood, the `placement` Components would be translated to Placement objects that contains cluster information.
The `deploy2clusters` Workflow Steps would trigger `deploy2clusters` Operator to read ClusterSelector objects via `target` name
and handles progapation of the app's resources to selected clusters.


### Case 2: Admin Policy

In this case, an initializer contains some admin policies, and developers specify an initializer to deploy apps.

Platform team would define an admin policy as a Component, and deploy such an environment as an Initializer:

```yaml
kind: Initializer
metadata:
  name: prod-env
spec:
  components:
    - name: prod-env-slo-policy
      type: slo-policy
      properties:
        properties:
          # slo indicates the service in this env must be 99.99% up
          slo: "99.99"
    - name: prod-env-chaos-policy
      type: chaos-policy
      properties:
        properties:
          io:
            action: fault
            volumePath: /var/run/etcd
            path: /var/run/etcd/**/*
            percent: 50
            duration: "400s"
```

Developers would specify a target to deploy via Workflow, which automatically does policy checks in the background:

```yaml
kind: Application
metadata:
  name: my-app
spec:
  workflow:
    - name: check-policy
      type: check-policy
      properties:
        # This app will be checked against the policies in the given target
        target: prod-env
        # If policy checks failed, send alerts to spcified channels
        notifications:
          slack: slack_url
          dingding: dingding_url
```

Under the hood, the `slo-policy` and `chaos-policy` Components would be translated to data objects (e.g. ConfigMap
or dedicated Policy objects) that contains policy data.
The `check-policy` Workflow Steps would trigger `check-policy` Operator to register the apps
to handlers of the target's policy checks, and handle notifications.


### case 3: Operator & CRD

In this case, an initializer defines the system Operators ands CRDs to be installed on a cluster.
Deploying such an initializer will setup these services and definitions on the host cluster.

Platform team would define these Operators and CRDs as Components, and deploy such an environment as an Initializer:

```yaml
kind: Initializer
metadata:
  name: prod-env
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

In this case, an Initializer defines shared services to be installed on the cluster.
Deploying such an initializer will setup these services on the host cluster.

Platform team would define the shared services as Components, and deploy such an environment as an Initializer:

```yaml
kind: Initializer
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

We will discuss the details of implementation plan in this section.

### Environment Controller

- Add a new Initializer CRD and controller skeleton in KubeVela.
- In reconcile logic:
  - If finalizer doesn't exist, add finalizer to the object.
  - If obj DeletionTimestamp is not zero (being deleted):
    - Wait until all contained component resources are deleted.
      In this way other remote resources (e.g. placement rules on other clusters) can be cleaned up entirely.
    - Then remote finalizer.
  - Check initializer dependency:
    - If any of them does not exist, try reconcile later.
  - Handle each resource:
    - Render the component into a resource object.
      Reuse and refactor existing logic.
    - If `mustOwn` is true, check if the resource ownerRef is the current Initializer CR.
    - Put current Initializer CR as an ownerRef into the object.
    - Apply the resource object.


## Considerations

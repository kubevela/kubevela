# Application-Level Policies and Customized Control-Logic Workflow Design

## Background

The current model consists of mainly Components and Traits. While this enables the Application object to plug-in operational capabilities, it is still no flexible enough. Specifically, it has the following limitations:

- The current control logic could not be customized. Once the Vela controller renders final k8s resources, it simply applies them without any extension points. In some scenarios, users want to do more complex operations like:
  - Blue-green upgrade old-new app revisions.
  - User interaction like manual approval/rollback.
  - Distributing workloads across multiple clusters.
  - Enforcing policies and auditting.
  - Pushing finalized k8s resources to Git for GitOps (via Flux/Argo) without applying the resources in Vela.
- There is no application-level config, but only per-component config. In some scenarios, users want to have app-level policies like:
  - Security: RBAC rules, audit settings, secret backend types.
  - Insights: app delivery lead time, frequence, MTTR.
  - GitOps: git repo, branch, credentials.

## Proposal

To resolve the aforementioned problems, we propose to add app-level policies and customizable workflow to Application API:

```yaml
kind: Application
spec:
  componnets: ...

  # Policies are rendered after components are rendered but before workflow are started
  policies:
    - type: security
      properties:
        rbac: ...
        audit: enabled
        secretBackend: vault

    - type: deployment-insights
      properties:
        leadTime: enabled
        frequency: enabled
        mttr: enabled

    - type: gitops
      properties:
        source: git-repo-url
        branch: main

  # workflow is used to customize the control logic.
  # If workflow is specified, Vela won't apply any resource, but provide rendered output in AppRevision.
  # workflow steps are executed in array order, and each step:
  # - will have a context in annotation.
  # - should mark "finish" phase in annotation.
  workflow:
  
    # blue-green rollout
    - type: blue-green-rollout
      stage: post-render # stage could be pre/post-render. Default is post-render.
      properties:
        partition: "50%"

    # traffic shift
    - type: traffic-shift
      properties:
        partition: "50%"

    # promote/rollback
    - type: rollout-promotion
      propertie:
        manualApproval: true
        rollbackIfNotApproved: true
```

This also implicates we will add two Definition CRDs -- `PolicyDefinition` and `WorkflowStepDefinition`:

```yaml
apiVersion: core.oam.dev/v1beta1
kind: PolicyDefinition
metadata:
  name: gitops
spec:
  schematic:
    cue:
      template: |
        output: {
          apiVersion: argoproj.io/v1alpha1
          kind: Application
          spec:
            source:
              repoURL: parameter.source
              targetRevision: HEAD
            ...
        }
        parameters: {
          source: string
        }

---
apiVersion: core.oam.dev/v1beta1
kind: WorkflowStepDefinition
spec:
  schematic:
    cue:
      template: ...
```

## Technical Details

To support policies and workflow, the application controller will be modified as the following:

- Before rendering the components, the controller will first execute the `stage: pre-render` steps.
- When generating AppRevision, the controller will put final resources (including policies) into a field `resources`:

  ```yaml
  kind: ApplicationRevision
  spec:
    ...
    resources:
      - raw:
          apiVersion: apps/v1
          kind: Deployment
          metadata:
            name: mysvc
          spec:
            replicas: 1
            ...
      - raw: ...
  ```
- If workflow is specified, the controller will then apply the ApplicationRevision, but not ApplicationContext. In this way, the resources won't be applied by Vela controller.
- The controller will then reconcile the workflow steps one-by-one. Each workflow step will be recorded in the Application.status:
  ```yaml
  kind: Application
  status:
    workflow:
    - type: rollout-promotion
      phase: running # succeeded | failed | stopped
      resourceRef:
        kind: Rollout
        name: ...
  ```

For each workflow step, it has the following interactions with the app controller:
- The controller will apply the workflow object with annotation `app.oam.dev/workflow-context`. This annotation will pass in the context marshalled in json defined as the following:
  ```go
  type WorkflowContext struct {
    AppName string
    AppRevisionName string
    WorkflowIndex int
  }
  ```
- The controller will wait for the workflow object's annotation `app.oam.dev/workflow-phase` to have value:
  - `succeeded`: the controller will run the next step.
  - `stopped`: the controller will stop the workflow.
  - `failed`: the controller will stop the workflow and report error status.

## Use Cases

In this section we will walk through how we implement workflow solutions for the following use cases.

### Case 1: Multi-cluster

In this case, users want to distribute workflow to multiple clusters. The dispatcher implementation is flexible and could be based on [open-cluster-management](https://open-cluster-management.io/) or other methods.

```yaml
workflow:
  - type: open-cluster-management
    properties:
      placement:
        - clusterSelector:
            region: east
          replicas: "70%"
        - clusterSelector:
            region: west
          replicas: "20%"
```

The process goes as:

- During infra setup, the Cluster objects are applied and agents are setup in each cluster to manage lifecycle of k8s clusters.
- Once the Application is applied, the OCM controller can retrieve all rendered resources from AppRevision. It will apply a ManifestWork object including all resources. Then the OCM agent will execute the workload creation in each cluster.

### Case 2: Blue-green rollout

In this case, users want to rollout a new version of the application components in a blue-green rolling upgrade style.

```yaml
workflow:
  # blue-green rollout
  - type: blue-green-rollout
    properties:
      partition: "50%"

  # traffic shift
  - type: traffic-shift
    properties:
      partition: "50%"

  # promote/rollback
  - type: rollout-promotion
    propertie:
      manualApproval: true
      rollbackIfNotApproved: true
```

The process goes as:

- By default, each modification of the Application object will generate an AppRevision object. The rollout controller will get the current revision from the context and retrieve the previous revision via kube API.
- Then the rollout controller will do the operation to rollings replicas between two revisions (the actual behavior depends on the workload type, e.g. Deployment or CloneSet).
- Once the rollover is done, the rollout controller can shift partial traffic to the new revision too.
- The rollout controller will wait for the manual approval. In this case, it is in the status of Rollout object:
  ```yaml
  kind: Rollout
  status:
    pause: true # change this to false
  ```

  The reference to the rollout object will be in the Application object:
  ```yaml
  apiVersion: core.oam.dev/v1beta1
  kind: Application
  status:
    workflow:
    - type: rollout-promotion
      resourceRef:
        kind: Rollout
        name: ...
  ```

### Case 3: Data Passing

In this case, users want to deploy a database component first, wait the database to be up and ready, and then deploy the application with database connection secret.

```yaml
components:
  - name: my-db
    type: mysql
    properties:

  - name: my-app
    type: webservice


workflow:
  - type: apply-component 
    properties:
      name: my-db

  # Wait for the MySQL object's status.connSecret to have value.
  - type: conditional-wait
    properties:
      resourceRef:
        apiVersion: database.example.org/v1alpha1
        kind: MySQLInstance
        name: my-db
      conditions:
        - field: status.connSecret
          op: NotEmpty

  # Patch my-app Deployment object's field with the secret name
  # emitted from MySQL object. And then apply my-app component.
  - type: apply-component 
    properties:
      name: my-app
      patch:
        to:
          field: spec.containers[0].envFrom[0].secretRef.name
        valueFrom:
          apiVersion: database.example.org/v1alpha1
          kind: MySQLInstance
          name: my-db
          field: status.connSecret

```

### Case 4: GitOps rollout

In this case, users just want Vela to provide final k8s resources and push them to Git, and then integrate with ArgoCD/Flux to do final rollout. Users will setup a GitOps workflow like below:


```yaml
policies:
- type: gitops # This part configures how ArgoCD/Flux watches the repo
  properties:
    source: git-repo-url
    branch: main

workflow:
- type: gitops # This part configures how to push resources to Git repo
  properties:
    gitRepo: git-repo-url
    branch: branch
    credentials: ...
```

The process goes as:

- The GitOps policy will be applied first, making ArgoCD/Flux watch the Git repo to sync any updates.
- Everytime an Appliation event is triggered, the GitOps workflow controller will push the rendered resources to a Git repo to trigger continuous deployment.

### Case 5: Template-based rollout

In this case, a template for Application object has already been defined. Instead of writing the `spec.components`, users will reference the template and provide parameters/patch to it.

```yaml
workflow:
  - type: helm-template
    stage: pre-render
    properties:
      source: git-repo-url
      path: chart/folder/path
      parameters:
        image: my-image
        replicas: 3
---
workflow:
  - type: kustomize-patch
    stage: pre-render
    properties:
      source: git-repo-url
      path: base/folder/path
      patch:
        spec:
          components:
            - name: instance
              properties:
                image: prod-image
```

The process goes as:

- On creating the application, app controller will apply the HelmTemplate/KustomizePatch objects, and wait for its status.
- The HelmTemplate/KustomizePatch controller would read the template from specified source, render the final config. It will compare the config with the Application object -- if there is difference, it will write back to the Application object per se.
- The update of Application will trigger another event, the app controller will apply the HelmTemplate/KustomizePatch objects with new context. But this time, the HelmTemplate/KustomizePatch controller will find no diff after the rendering. So it will skip this time.

## Considerations



# Making Environments For Application Deployment

## Background

An application development team usually needs to initialize some shared environments for developers to deploy applications to.
For example, a team would initialize two environments, a *dev* environment for testing applications,
and a *prod* environment for running applications to serve live traffic.

The environment is a logical concept grouping common resources that many applications would depend on. Below are a list of cases:

- Kubernetes clusters. These include not only existing clusters but also new clusters to create during environment setup.
- Admin policies. Production environment would usually set global policies for all application deployments,
  like chaos testing, SLO requirements, security scanning, misconfiguration detection.
- System components. That includes installing system level Operators & CRDS like FluxCD, KEDA, OpenKruise,
  shared Namespaces, Observability (Prometheus, Grafana, Loki), Terraform Controller, KubeFlow Controller, etc.
- Shared services. Environments contains a variety of system services that are be shared by applications.
  These resources can include DB, cache, load balancers, API Gateways, etc.

In this doc, we will provide solutions and case studies on how to use KubeVela to make environments, initialize shared resources,
and compose application rollout across multiple environments.


## Using KubeVela to initialize Environment

An environment is just a logical concept that groups a bundle of resources for multiple applications to share.
To make an environment, we first need to initialize the shared resources.

When initializing an environment, there is one additional requirement:

- The ability to describe dependencies between environment modules. For example, both *dev* and *prod* environments
  would depend on a common module that installs operators both *dev* and *prod* would use.

To setup shared resources and define dependencies, we propose to use Application with two Workflow Steps
to achieve the requirements. An example looks like below:

```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: setup-dev-env
spec:
  # Using Application to deploy resources that makes up an environment

  components:
  - name: <env-component-name>
    type: <env-component-type>
    properties:
      <env-component-props>

  policies:
  - name: <env-policy-name>
    type: <env-policy-type>
    properties:
      <env-policy-props>

  workflow:
  - name: wait-dependencies
    # depends-on-app step will wait until the dependent app's status turns to Running phase
    type: depends-on-app
    properties:
      name: <application name>
      namespace: <application namespace>
      
  - name: apply-self
    # apply-application will apply all components of this Application per se
    type: apply-application
```


## Using KubeVela to compose multi-env application rollout

Once we have initialized an environment, we can then deploy applications across environments.
For example, we can deploy an application to dev env first, then verify the app is working, and promote to prod env at the end.

We can use the following Definition to achieve that:

- env-binding Policy: This defines the config patch and placement strategy per env.
- deploy2env WorkflowStep: This picks which policy and env to deploy the app to.
- suspend WorkflowStep: This will pause the workflow for some manual validation.

Below is an example:

```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: multi-env-demo
spec:
  components:
    - name: myimage-server
      type: webservice
      properties:
        image: myimage:v1.1
        port: 80

  policies:
    - name: my-binding-policy
      type: env-binding
      properties:
        envs:
          - name: test

            patch: # overlay patch on above components
              components:
                - name: myimage-server
                  type: webservice
                  properties:
                    image: myimage:v1.2
                    port: 80

            placement: # selecting the cluster to deploy to
              clusterSelector:
                labels:
                  purpose: test

          - name: prod
            placement:
              clusterSelector:
                labels:
                  purpose: prod

  workflow:
    steps:
      - name: deploy-test-env
        type: deploy2env
        properties:
          policy: my-binding-policy
          env: test

      - name: manual-approval 
        type: suspend

      - name: deploy-prod-env
        type: deploy2env
        properties:
          policy: my-binding-policy
          env: prod
```

Here're more details for above example:

- It sets up an `env-binding` policy which defines two envs for users to use.
  In each env, it defines the config patch and placement strategy specific to this env.
- When the application runs, it triggers the following workflow:
  - First it picks the policy, and picks the `test` env which is also defined inside the policy.
  - Then the `deploy2env` step will load the policy data, picks the `test` env specific config section.
    This step will render the final Application with patch data,
    and picks the cluster to deploy to based on given placement strategy,
    and finally deploys the Application to the cluster.
  - Then it runs `suspend` step, which acts as an approval gate until user validation 
  - Finally, it runs `deploy2env` step again. Only this time it picks the `prod` env.
    This step will render the final Application with patch data,
    and picks the cluster to deploy to based on given placement strategy,
    and finally deploys the Application to the cluster.


## Implementation Plan

We will discuss the details of implementation plan in this section.

## Considerations


## Appendix

### Creating Kubernetes cluster

In this section, we will show how to create a Kubernetes cluster on Alibaba Cloud using Terraform.

We will first setup Terraform Alibab provider:

```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: terraform-alibaba
  namespace: vela-system
spec:
  components:
    - name: default
      type: raw
      properties:
        apiVersion: terraform.core.oam.dev/v1beta1
        kind: Provider
        metadata:
          namespace: default
        spec:
          provider: alibaba
          region: cn-hongkong
          credentials:
            source: Secret
            secretRef:
              namespace: vela-system
              name: alibaba-account-creds
              key: credentials
  workflow:
  - name: wait-dependencies
    type: depends-on-app
    properties:
      name: terraform
      namespace: vela-system
      
  - name: apply-self
    type: apply-application
```

Then we will create a Kubernetes cluster:

```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: managed-cluster
  namespace: vela-system
spec:
  components:
    - name: ack-worker
      type: alibaba-ack
      properties:
        writeConnectionSecretToRef:
          name: ack-conn
          namespace: vela-system
  workflow:
    steps:
      - name: wait-dependencies
        type: depends-on-app
        properties:
          name: terraform-alibaba
          namespace: vela-system

      - name: wait-dependencies
        type: depends-on-app
        properties:
          name: ocm-cluster-manager
          namespace: vela-system
          
      - name: terraform-ack
        type: create-ack
        properties:
          component: ack-worker
        outputs:
          - name: connInfo
            valueFrom: connInfo

      - name: register-ack
        type: register-cluster
        inputs:
          - from: connInfo
            parameterKey: connInfo
        properties:
          # user should set public network address of APIServer
          hubAPIServer: {{ public network address of APIServer }}
          env: prod
          initNameSpace: default
          patchLabels:
            purpose: test
```

It will wait for dependent system components to be installed first, and then create the cluster,
and finally register the cluster in K8s API.

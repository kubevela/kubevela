# Building Machine Learning Platforms Using KubeVela and ACK


## Background

Data scientists are embracing Kubernetes as the infrastructure to run ML apps.
Nonetheless, when it comes to converting machine learning code to application delivery pipelines, data scientists struggle a lot --
It is a very challenging, time-consuming task, and needs the cooperation of different domain experts: Application Developer, Data Scientist, Platform Engineer.

As a result, platform teams are building self-service ML platforms for data scientists to test, deploy and upgrade models.
Such platforms provide the following benefits:

- Improve the speed-to-market for ML models.
- Lower the barrier to entry for ML developers to get their models into production.
- Implement operational efficiencies and economies of scale.

With KubeVela and ACK (Alibaba Kubernetes Service), we can build ML platforms easily:

- ACK + Alibaba Cloud can provide infra services to support deployment of ML code and models.
- KubeVela can provide standard workflow and APIs to glue all the deployment steps.

In this doc, we will discuss one generic solution to building a ML platform using KubeVela and ACK.
We will see that by using KubeVela it is easy to build high-level abstractions and developer-facing APIs to improve user experience on top of cloud infrastructure.


## ACK Features Used

Buidling ML Platforms with KubeVela on ACK gives you the following feature benefits:

- You can provision and manage Kubernetes clusters via ACK console and easily configure multiple compute and GPU node configurations.
- You can scale up cluster resources or setup staging environments in pay-as-you-go mode by using ASK (Serverless Kubernetes).
- You can deploy your apps to the edge and manage them in edge-autonomous mode by using ACK@Edge.
- Machine learning jobs can share GPUs to save cost and improve utilization by enabling GPU sharing mode on ACK.
- Centralized and unified application logs/metrics in ARMS, which helps with monitoring, troubleshooting, debugging.


## Initialize Infrastructure Environment

Users need to setup the following infrastructure resources before deploying ML code.

- Kubernetes cluster
- Kubeflow operator
- OSS bucket

We propose to add the following Initializer to achieve it:

```yaml
apiVersion: core.oam.dev/v1beta1
kind: Initializer
spec:
  appTemplate:
    spec:
      components:
        - name: prd-cluster
          type: k8s-cluster
          properties:
            provider: alibaba
            resource: ACK
            version: v1.20

        - name: dev-cluster
          type: k8s-cluster
          properties:
            provider: alibaba
            resource: ASK
            version: v1.20

        - name: kubeflow
          type: helm-chart
          properties:
            repo: repo-url
            chart: kubeflow
            namespace: kubeflow-system
            create-nmespace: true

        - name: s3-bucket
          type: s3-bucket
          properties:
            provider: alibaba
            bucket: ml-example


      workflows:
        - name: create-prod-cluster
          type: terraform-apply
          properties:
            component: prod-cluster

        - name: create-dev-cluster
          type: terraform-apply
          properties:
            component: prod-cluster

        - name: deploy-kubeflow
          type: helm-apply
          properties:
            component: kubeflow

        - name: create-s3-bucket
          type: terraform-apply
          prooperties:
            component: s3-bucket
```

## Model Training and Serving

In this section, we will define the high-level, user-facing APIs exposed to users.

Here is an overview:

```yaml
kind: Application
spec:
  components:
    # This is the component to train the models.
    - name: my-tfjob
      type: tfjob

      properties:
        # modelVersion defines the location where the model is stored.
        modelVersion:
          modelName: mymodel
          # The dockerhub repo to push the generated image
          imageRepo: myhub/mymodel
        # tfReplicaSpecs defines the config to run the training job
        tfReplicaSpecs:
          Worker:
            replicas: 3
            template:
              spec:
                containers:
                  - name: tensorflow
                    image: tf-mnist-estimator-api:v0.1

    # This is the component to serve the models.
    - name: my-tfserving-prod
      type: tfserving

      properties:
        # Below we show two predictors that splits the serving traffic
        predictors:
           # 90% traffic will be roted to this predictor.
          - name: model-a-predictor
            modelVersion: mymodel-v1
            replicas: 3
            trafficPercentage: 90
            autoScale:
              minReplicas: 1
              maxReplicas: 10
            batching:
              batchSize: 32
            template:
              spec:
                containers:
                - name: tensorflow
                  image: tensorflow/serving:1.11.0
          # 10% traffic will be roted to this predictor.
          - name: model-b-predictor
            modelVersion: mymodel-v2
            replicas: 3
            trafficPercentage: 10
            autoScale:
              minReplicas: 1
              maxReplicas: 10
            batching:
              batchSize: 64
            template:
              spec:
                containers:
                - name: tensorflow
                  image: tensorflow/serving:1.11.1

      traits:
        - name: metrics
          type: arms-metrics
          
        - name: logging
          type: arms-logging


    # This is the component to serve the models.
    - name: my-tfserving-dev
      type: tfserving
      properties:
        predictors:
          - name: model-predictor
            modelVersion: mymodel-v2
            template:
              spec:
                containers:
                - name: tensorflow
                  image: tensorflow/serving:1.11.1


  workflow:
    steps:
      - name: train-model
        type: ml-model-training
        properties:
          component: my-tfjob
          # The workflow task will load the dataset into the volumes of the training job container
          dataset:
            s3:
              bucket: bucket-url

      # wait for user to evaluate and decide to pass/fail
      - name: evaluate-model
        type: suspend

      - name: save-model
        type: ml-model-checkpoint
        properties:
          # modelVersion defines the location where the model is stored.
          modelVersion:
            modelName: mymodel-v2
            # The docker repo to push the generated image
            imageRepo: myrepo/mymodel
      
      - name: serve-model-in-dev
        type: ml-model-serving
        properties:
          component: my-tfserving-dev
          env: dev

      # wait for user to evaluate and decide to pass/fail
      - name: evaluate-serving
        type: suspend

      - name: serve-model-in-prod
        type: ml-model-serving
        properties:
          component: my-tfserving-prod
          env: prod
```

## Integration with ACK Services

In above we have defined the user APIs.
Under the hood, we can leverage ACK and cloud services to support the deployment of the ML models.
Here are how they are implemented:

- We can create and manage ACK clusters in the `create-cluster` workflow task.
  We can define the ACK cluster templates in `k8s-cluster` component.
- We can use ASK as cluster resources for dev environment, which is defined in `dev-cluster` component.
  Once users have evaluated the service and promoted to production, the ASK cluster will automatically scale down.
- We can use ASK for scaling up cluster resources in prod environment.
  When traffic spike comes, users would have more resources automatically to create more serving instances,
  which keeps the services responsive.
- We can deploy ML models to ACK@Edge to keep services running on edge-autonomous mode.
- We can provide GPU sharing options to users by using ACK GPU sharing feature.
- We can export the logs and metrics to ARMS and display them in dashboard automatically.


## Considerations

### 1. Comparison to using Kubeflow

How is it different from traditional methods like using Kubeflow directly:

- Users using Kubeflow still needs to write a lot of scripts.
  It is a challenging problem to manage those scripts.
  For example, how to store them, and how to document them?
- With KubeVela, we provide a standard way to manage the these glue code.
  They are managed in modules, stored as CRDs, and exposed in CUE APIs.
- Kubeflow and Kubeflow works in different levels.
  Kubeflow provides low-level, atomic capabilities.
  KubeVela works on higher-level APIs to simplify deployment and operations for users.

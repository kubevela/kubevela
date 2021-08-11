This example shows how to use imagePullJob as a workflow step in an application deployment.

## Install Definitions

  install openkruise:
    ```
    vela addon enable kruise
    ```
  or install openkruise manully
    ```
    helm install kruise https://github.com/openkruise/kruise/releases/download/v0.9.0/kruise-chart.tgz --set featureGates="PreDownloadImageForInPlaceUpdate=true"
    kubectl apply -f kubevela/vela-templates/addons/kruise/definitions/imagepulljob-step.yaml
    ```

  Pre-download workflow step should be installed successfully at present. Check Component and Workflow definitions:
    ```
    kubectl get workflowstep
    ```

  Output:
    ```
    NAME              AGE
    predownloadimage  49s
    ```

  Install another workflow step that need to be performed after pre-download image:
    ```
    kubectl apply -f applydemo-definition.yaml
    ```

  apply(Nginx) workflow step should be installed successfully at present. Check Component and Workflow definitions:
    ```
    kubectl get workflowstep
    ```

  Output:
    ```
    NAME              AGE
    predownloadimage  49s
    apply             49s
    ```


## Begin The Workflow Demo

This Demo is to apply pull image for nginx first and that start a nginx application.


1. Apply Application:
    ```
    kubectl apply -f basic_image_pull.yaml
    ```


2. Check workflow status in Application:
    ```
    kubectl get application pullimage-sample -o yaml
    ```

    Output:
    ```yaml
    ...
    apiVersion: core.oam.dev/v1beta1
    kind: Application
    metadata:
      ...
    spec:
      components:
      - name: nginx
        properties:
          image: nginx:1.9.1
          port: 80
        type: webservice
      workflow:
        steps:
        - name: pullimage
          properties:
            image: nginx:1.9.1
            kvs:
              kubernetes.io/os: linux
            parallel: 3
          type: predownloadimage
        - name: deploy-remaining
          properties:
            component: nginx
          type: apply
    status:
      ...
      services:
      - healthy: true
        name: nginx
        workloadDefinition:
          apiVersion: apps/v1
          kind: Deployment
      status: running
      workflow:
        appRevision: pullimage-sample-v1
        contextBackend:
          kind: ConfigMap
          name: workflow-pullimage-sample-v1
        stepIndex: 2
        steps:
        - name: pullimage
          phase: succeeded
          resourceRef: {}
          type: predownloadimage
        - name: deploy-remaining
          phase: succeeded
          resourceRef: {}
          type: apply
        suspend: false
        terminated: true
      ```
    In Above "workflow:" fields. you can see that two steps all in phase succeeded, which means these two steps have been performed successfully.

2. Check Resource in cluster.

    ```
    kubectl get imagepulljob
    kubectl get pods
    ```

    Output:

    ```
    NAME             TOTAL   ACTIVE   SUCCEED   FAILED   AGE   MESSAGE
    pull-image-job   3       0        3         0        19s   job is running, progress 0.0%
    ```

    ```
    NAME                      READY   STATUS    RESTARTS   AGE
    nginx-868d6c9dc7-kmgvs    1/1     Running   0          55s
    ```

  The above step shows how to use workflow step to start a deployment.
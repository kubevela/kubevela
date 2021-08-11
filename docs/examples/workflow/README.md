## Install Definitions

   ```
   kubectl apply -f definition.yaml
   ```
 Check Component and Workflow definitions:

   ```
    kubectl get componentDefinition
    kubectl get workflowstep
   ```

 Output:
   ```
    NAME              AGE
    singleton-server   49s

    NAME              AGE
    apply-component   49s
    apply-with-ip      49s
   ```


## Begin The Workflow Demo

This Demo is to apply component in the cluster in order by workflow, and inject the IP of the previous pod into the environment variables of the next Pod.


1. Apply Application:

    ```
    kubectl apply -f app.yaml
    ```



2. Check workflow status in Application:

    ```
    kubectl get -f app.yaml
    ```

    Output:
    ```yaml
    ...
    status:  
      workflow:
        appRevision: application-sample-v1
        contextBackend:
          apiVersion: v1
          kind: ConfigMap
          name: workflow-application-sample-v1
          uid: 783769c9-0fe1-4686-8528-94ce2887a5f8
        stepIndex: 2
        steps:
        - name: deploy-server1
          phase: succeeded
          resourceRef:
            apiVersion: ""
            kind: ""
            name: ""
          type: apply
        - name: deploy-server2
          phase: succeeded
          resourceRef:
            apiVersion: ""
            kind: ""
            name: ""
          type: apply

      ```

2. Check Resource in cluster.

    ```
    kubectl get pods
    ```

    Output:

    ```
    NAME       READY   STATUS    RESTARTS   AGE
    server1    1/1     Running   0          15s
    server2    1/1     Running   0          18s
    ```

    This means the resource has been rendered correctly.


3. Check `server2` Environment variable

    ```
    kubectl exec server2 -- env|grep PrefixIP
    ```

    Output:

    ```
    PrefixIP=10.244.0.22
    ```
## WorkflowStep Definition Introduction.

WorkflowStep consists of a series of actions, you can describe the actions to be done  step by step in WorkflowStep Definition.

1. `op.#Load`
   Get component schema from workflow context
2. `op.#Apply`
   Apply schema to cluster.
3. `op.#ConditionalWait`
   Condition waits until continue is true.
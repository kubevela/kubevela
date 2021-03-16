# Rollout Example
Here is an example of how to rollout an application with a component of type deployment. 


## Rollout steps
1. Install deployment based workloadDefinition
```shell
kubectl apply -f docs/examples/deployment-rollout/webservice-definition.yaml
```

2. Apply an application 
```shell
kubectl apply -f docs/examples/deployment-rollout/app-source.yaml
```
Wait for the application's status to be "running"

3. Prepare the application for rolling out
```shell
kubectl apply -f docs/examples/deployment-rollout/app-source-prep.yaml
```
Wait for the applicationConfiguration "test-rolling-v1" `Rolling Status` to be "RollingTemplated"

4. Modify the application image and apply
```shell
kubectl apply -f docs/examples/deployment-rollout/app-target.yaml
```
Wait for the applicationConfiguration "test-rolling-v2" `Rolling Status` to be "RollingTemplated"

5. Mark the application as normal
```shell
kubectl apply -f docs/examples/deployment-rollout/app-target-done.yaml
```

6. Apply the application deployment with pause
```shell
kubectl apply -f docs/examples/deployment-rollout/app-deploy-pause.yaml
```
Check the status of the ApplicationDeployment and see the step by step rolling out.
This rollout will pause after the second batch.

7. Apply the application deployment that completes the rollout
```shell
kubectl apply -f docs/examples/deployment-rollout/app-deploy-finish.yaml
```
Check the status of the ApplicationDeployment and see the rollout completes, and the 
applicationDeployment's "Rolling State" becomes `rolloutSucceed`
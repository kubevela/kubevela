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

3. Modify the application image and apply
```shell
kubectl apply -f docs/examples/deployment-rollout/app-target.yaml
```

4. Apply the application deployment with pause
```shell
kubectl apply -f docs/examples/deployment-rollout/app-rollout-pause.yaml
```
Check the status of the ApplicationRollout and see the step by step rolling out.
This rollout will pause after the second batch.

7. Apply the application deployment that completes the rollout
```shell
kubectl apply -f docs/examples/deployment-rollout/app-rollout-finish.yaml
```
Check the status of the ApplicationRollout and see the rollout completes, and the 
ApplicationRollout's "Rolling State" becomes `rolloutSucceed`
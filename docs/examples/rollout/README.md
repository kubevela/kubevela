# Rollout Example
Here is an example of how to rollout an application with a component of type CloneSet. 

## Install Kruise
```shell 
helm install kruise https://github.com/openkruise/kruise/releases/download/v0.7.0/kruise-chart.tgz
```

## Rollout steps
1. Install CloneSet based workloadDefinition
```shell
kubectl apply -f docs/examples/rollout/clonesetDefinition.yaml
```

2. Apply an application 
```shell
kubectl apply -f docs/examples/rollout/app-source.yaml
```
Wait for the application's status to be "running"

3. Prepare the application for rolling out
```shell
kubectl apply -f docs/examples/rollout/app-source-prep.yaml
```
Wait for the applicationConfiguration "test-rolling-v1" `Rolling Status` to be "RollingTemplated"

4. Modify the application image and apply
```shell
kubectl apply -f docs/examples/rollout/app-target.yaml
```
Wait for the applicationConfiguration "test-rolling-v2" `Rolling Status` to be "RollingTemplated"

5. Apply the application deployment with pause
```shell
kubectl apply -f docs/examples/rollout/app-deploy-pause.yaml
```
Check the status of the ApplicationDeployment and see the step by step rolling out.
This rollout will pause after the second batch.

5. Apply the application deployment that completes the rollout
```shell
kubectl apply -f docs/examples/rollout/app-deploy-finish.yaml
```
Check the status of the ApplicationDeployment and see the rollout completes and the 
applicationDeployment's "Rolling State" becomes `rolloutSucceed`

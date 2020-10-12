# Quick start

This example show case how one can use a flagger trait to rollout a component with 
 
## Install Vela core
```shell script
make
bin/vela install
```

## Run ApplicationConfiguration
```shell script
kubectl apply -f documentation/samples/rollout-demo/definitions.yaml
traitdefinition.core.oam.dev/canaries.flagger.app created
traitdefinition.core.oam.dev/ingresses.extensions created
workloaddefinition.core.oam.dev/deployments.apps created

kubectl apply -f documentation/samples/rollout-demo/deploy-component.yaml
component.core.oam.dev/rollout-demo-app created

kubectl apply -f documentation/samples/rollout-demo/appConfig-rollout.yaml
applicationconfiguration.core.oam.dev/sample-application-rollout created
```

## Verify that both the canary and primary deployments are created
```shell script
kubectl get pod
NAME                                        READY   STATUS    RESTARTS   AGE
rollout-demo-app-9d58fd6d-54jnz             1/1     Running   0          23s
rollout-demo-app-9d58fd6d-w8pjj             1/1     Running   0          23s
rollout-demo-app-primary-77486f6d86-t6slm   1/1     Running   0          13s
rollout-demo-app-primary-77486f6d86-tk4hb   1/1     Running   0          13s

kubectl get svc
NAME                       TYPE        CLUSTER-IP      EXTERNAL-IP   PORT(S)   AGE
rollout-demo-app           ClusterIP   10.106.150.0    <none>        80/TCP    10s
rollout-demo-app-canary    ClusterIP   10.106.230.14   <none>        80/TCP    12s
rollout-demo-app-primary   ClusterIP   10.103.129.11   <none>        80/TCP    1s
```

## Verify the version of the canary and primary deployment
```shell script
kubectl get deploy rollout-demo-app -o=jsonpath="{.spec.template.spec.containers[0].image}"
luxas/autoscale-demo:v0.1.0

kubectl get deploy rollout-demo-app-primary -o=jsonpath="{.spec.template.spec.containers[0].image}"
luxas/autoscale-demo:v0.1.0
```

## Upgrade the component to v0.1.1
```shell script
sed -i .bak  's/0.1.0/0.1.1/g' documentation/samples/rollout-demo/deploy-component.yaml
kubectl apply -f documentation/samples/rollout-demo/deploy-component.yaml
```

## Verify the version of the canary deployment is upgraded, but the primary is not yet
```shell script
kubectl get deploy rollout-demo-app -o=jsonpath="{.spec.template.spec.containers[0].image}"
luxas/autoscale-demo:v0.1.1

kubectl get deploy rollout-demo-app-primary -o=jsonpath="{.spec.template.spec.containers[0].image}"
luxas/autoscale-demo:v0.1.0
```

## Observe that the rollout through the canary object
```shell script
kubectl describe canary
...
...
Events:
  Type    Reason  Age    From     Message
  ----    ------  ----   ----     -------
   Normal  Synced  3m35s  flagger  New revision detected! Scaling up rollout-demo-app.default
   Normal  Synced  3m5s   flagger  Starting canary analysis for rollout-demo-app.default
   Normal  Synced  3m5s   flagger  Advance rollout-demo-app.default canary weight 10
   Normal  Synced  2m35s  flagger  Advance rollout-demo-app.default canary weight 20
   Normal  Synced  2m5s   flagger  Advance rollout-demo-app.default canary weight 30
   Normal  Synced  95s    flagger  Advance rollout-demo-app.default canary weight 40
   Normal  Synced  65s    flagger  Advance rollout-demo-app.default canary weight 50
   Normal  Synced  35s    flagger  Copying rollout-demo-app.default template spec to rollout-demo-app-primary.default
   Normal  Synced  5s     flagger  Advance rollout-demo-app.default primary weight 100
   Normal  Synced  1s     flagger  Promotion completed! Scaling down rollout-demo-app.default
```

## Verify that the prdeployment is upgraded, but the primary is not yet
```shell script
kubectl get deploy rollout-demo-app -o=jsonpath="{.spec.template.spec.containers[0].image}"
luxas/autoscale-demo:v0.1.1

kubectl get deploy rollout-demo-app-primary -o=jsonpath="{.spec.template.spec.containers[0].image}"
luxas/autoscale-demo:v0.1.0
```

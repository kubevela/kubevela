# Quick start

This example show case how one can use a flagger trait to rollout a component with 

## Install OAM flagger
```shell script
make
helm install -n flagger rollout charts/flagger/
```

## Install Vela core
```shell script
make
bin/vela install
```

## Run ApplicationConfiguration V1
```shell script
kubectl apply -f e2e/raw-objects/samples/rollout-demo/definitions.yaml
traitdefinition.core.oam.dev/canaries.flagger.app created
traitdefinition.core.oam.dev/ingresses.extensions created
workloaddefinition.core.oam.dev/deployments.apps created

kubectl apply -f e2e/raw-objects/samples/rollout-demo/deploy-component-v1.yaml
component.core.oam.dev/rollout-demo-app created

kubectl apply -f e2e/raw-objects/samples/rollout-demo/appConfig-rollout-v1.yaml
applicationconfiguration.core.oam.dev/sample-application-rollout created
```

## Verify that revision workload is created
```shell script
kubectl get controllerrevision
NAME                  CONTROLLER                                REVISION   AGE
rollout-demo-app-v1   component.core.oam.dev/rollout-demo-app   1          64s

kubectl get deploy
NAME                  READY   UP-TO-DATE   AVAILABLE   AGE
rollout-demo-app-v1    1/1    1            1           80s
```

## Upgrade the component to v2 and change the revision name of the component in the appConfig 
```shell script
kubectl apply -f e2e/raw-objects/samples/rollout-demo/deploy-component-v2.yaml
component.core.oam.dev/rollout-demo-app configured

kubectl apply -f e2e/raw-objects/samples/rollout-demo/appConfig-rollout-v2.yaml
applicationconfiguration.core.oam.dev/sample-application-rollout created
```

## Verify that new revision workload is created and the workloads
```shell script
kubectl get controllerrevision
NAME                  CONTROLLER                                REVISION   AGE
rollout-demo-app-v1   component.core.oam.dev/rollout-demo-app   1          90s
rollout-demo-app-v2   component.core.oam.dev/rollout-demo-app   2          5s
```

## Verify that we have created services that select all pods in all the revisions
```shell script
kubectl get svc
NAME                       TYPE        CLUSTER-IP       EXTERNAL-IP   PORT(S)   AGE
rollout-demo-app           ClusterIP   10.103.185.101   <none>        80/TCP    2m52s
rollout-demo-app-canary    ClusterIP   10.99.87.149     <none>        80/TCP    2m22s
rollout-demo-app-primary   ClusterIP   10.102.31.57     <none>        80/TCP    2m22s

kubectl describe svc rollout-demo-app  
Name:              rollout-demo-app
Namespace:         default
Labels:            app=rollout-demo-app
Annotations:       <none>
Selector:          app=rollout-demo-app
Type:              ClusterIP
IP:                10.103.185.101
Port:              http  80/TCP
TargetPort:        8080/TCP
Endpoints:         172.17.0.13:8080,172.17.0.14:8080,172.17.0.15:8080 + 4 more...
Session Affinity:  None
Events:            <none>
```

## Verify the version of the canary and primary deployment are what is intended
```shell script
kubectl get deploy rollout-demo-app-v1 -o=jsonpath="{.spec.template.spec.containers[0].image}"
stefanprodan/podinfo:4.0.6

kubectl get deploy rollout-demo-app-v2 -o=jsonpath="{.spec.template.spec.containers[0].image}"
stefanprodan/podinfo:5.0.2
```

## Observe the flagger canary 
```shell script
kubectl get canary --watch
NAME                STATUS        WEIGHT   LASTTRANSITIONTIME
rollout-demo-app    Initializing   0        2020-10-24T10:59:56Z
rollout-demo-app    Initializing   0        2020-10-24T10:59:56Z
rollout-demo-app    Initialized    0        2020-10-24T11:00:54Z
rollout-demo-app    Progressing    0        2020-10-24T11:01:24Z
rollout-demo-app    Progressing    10       2020-10-24T11:01:54Z
rollout-demo-app    Progressing    20       2020-10-24T11:02:24Z
rollout-demo-app    Progressing    30       2020-10-24T11:02:54Z
rollout-demo-app    Progressing    40       2020-10-24T11:03:24Z
rollout-demo-app    Progressing    50       2020-10-24T11:03:54Z
rollout-demo-app    Promoting      0        2020-10-24T11:04:24Z
rollout-demo-app    Promoting      100      2020-10-24T11:04:54Z
rollout-demo-app    Finalising     0        2020-10-24T11:05:24Z
```

## Observe that the rollout through the canary object, you shall see something like this
```shell script
kubectl describe canary
...
...
Events:
  Type     Reason  Age                 From     Message
  ----     ------  ----                ----     -------
  Warning  Synced  5m30s               flagger  waiting for rollout to finish: observed deployment generation less then desired generation
  Normal   Synced  5m (x2 over 5m30s)  flagger  all the metrics providers are available!
  Normal   Synced  5m                  flagger  Initialization done! rollout-demo-app.default
  Normal   Synced  4m30s               flagger  New revision detected! Scaling up rollout-demo-app-v2.default
  Normal   Synced  4m                  flagger  Starting canary analysis for rollout-demo-app-v2.default
  Normal   Synced  4m                  flagger  Advance rollout-demo-app.default canary weight 10
  Normal   Synced  3m30s               flagger  Advance rollout-demo-app.default canary weight 20
  Normal   Synced  3m                  flagger  Advance rollout-demo-app.default canary weight 30
  Normal   Synced  2m30s               flagger  Advance rollout-demo-app.default canary weight 40
  Normal   Synced  2m                  flagger  Advance rollout-demo-app.default canary weight 50
  Normal   Synced  30s                 flagger  Promote rollout-demo-app.default
  Normal   Synced  30s                 flagger  Promoting the traffic to the new targe in one shot
  Normal   Synced  0s (x3 over 60s)    flagger  (combined from similar events): Promotion completed!

```

## Verify that the instances of the v2 workload is the same as the canary maxReplica
```shell script
kubectl get deploy
NAME                  READY   UP-TO-DATE   AVAILABLE   AGE
rollout-demo-app-v1   0/0     0            0           15m
rollout-demo-app-v2   7/7     7            7           15m
```

## Roll back to V1
```shell script
kubectl apply -f e2e/raw-objects/samples/rollout-demo/appConfig-rollback-v1.yaml
applicationconfiguration.core.oam.dev/sample-application-rollout configured
```

## Observe the flagger canary rollback
```shell script
kubectl get canary --watch
NAME                STATUS        WEIGHT   LASTTRANSITIONTIME
rollback-demo-app   Initialized   0        2020-10-24T11:17:04Z
rollback-demo-app   Progressing   0        2020-10-24T11:17:34Z
rollback-demo-app   Progressing   10       2020-10-24T11:18:04Z
rollback-demo-app   Progressing   20       2020-10-24T11:18:34Z
rollback-demo-app   Progressing   30       2020-10-24T11:19:04Z
rollback-demo-app   Progressing   40       2020-10-24T11:19:34Z
rollback-demo-app   Progressing   50       2020-10-24T11:20:04Z
rollback-demo-app   Promoting     0        2020-10-24T11:20:34Z
rollback-demo-app   Promoting     100      2020-10-24T11:21:04Z
rollback-demo-app   Finalising    0        2020-10-24T11:21:34Z
rollback-demo-app   Succeeded     0        2020-10-24T11:22:04Z
```

## Verify that the instances of the v1 workload is the same as the canary maxReplica
```shell script
kubectl get deploy
NAME                  READY   UP-TO-DATE   AVAILABLE   AGE
rollout-demo-app-v1   7/7     7            7           24m
rollout-demo-app-v2   0/0     0            0           24m
```

## Clean up
```shell script
kubectl delete -f e2e/raw-objects/samples/rollout-demo/ 
```

## Improvements
1. The current component garbage collector uses a very unreliable way to figour out if a workload is a revisioned one that needs to keep or GCed. (basically to see if its name starts with componentName-)
2. The workload name will not be component-v when the user actually put a name in the component. This will lead to problems in the above case
3. Modify the primary/canary service so that they point to the primary/canary resources individually
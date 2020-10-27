# cron type Autoscaler

- Apply manifest
```
$ kubectl apply -f standard_v1alpha2_autoscaler.yaml

$ kubectl describe scaledobjects.keda.sh example-scaler
Name:         example-scaler
Namespace:    default
Labels:       scaledObjectName=example-scaler
Annotations:  <none>
API Version:  keda.sh/v1alpha1
Kind:         ScaledObject
Metadata:
  Creation Timestamp:  2020-09-28T09:47:11Z
  Finalizers:
    finalizer.keda.sh
  Generation:  1
  Owner References:
    API Version:           standard.oam.dev/v1alpha1
    Block Owner Deletion:  true
    Controller:            true
    Kind:                  Autoscaler
    Name:                  example-scaler
    UID:                   8ae85eb2-6f1c-4d9e-892c-6af22fa2fac5
  Resource Version:        478397
  Self Link:               /apis/keda.sh/v1alpha1/namespaces/default/scaledobjects/example-scaler
  UID:                     6c02a685-92e4-4667-84f5-c9d781385cbf
Spec:
  Max Replica Count:  4
  Min Replica Count:  2
  Scale Target Ref:
    Name:  php-apache
  Triggers:
    Metadata:
      Desired Replicas:  4
      End:               48 19 * * 1
      Start:             48 17 * * 1
      Timezone:          Asia/Shanghai
    Name:                weekend-cron
    Type:                cron
    Metadata:
      Desired Replicas:  4
      End:               48 19 * * 6
      Start:             48 17 * * 6
      Timezone:          Asia/Shanghai
    Name:                weekend-cron
    Type:                cron
Status:
  Conditions:
    Message:  ScaledObject is defined correctly and is ready for scaling
    Reason:   ScaledObjectReady
    Status:   True
    Type:     Ready
    Message:  Scaling is not performed because triggers are not active
    Reason:   ScalerNotActive
    Status:   False
    Type:     Active
  External Metric Names:
    cron-Asia-Shanghai-4817xx1-4819xx1
    cron-Asia-Shanghai-4817xx6-4819xx6
  Original Replica Count:  3
  Scale Target GVKR:
    Group:            apps
    Kind:             Deployment
    Resource:         deployments
    Version:          v1
  Scale Target Kind:  apps/v1.Deployment
Events:               <none>
```

- Monitor KEDA ScaledObject and target deployment
```
$ kubectl get scaledobjects.keda.sh --watch
NAME             SCALETARGETKIND   SCALETARGETNAME   TRIGGERS   AUTHENTICATION   READY   ACTIVE   AGE
example-scaler                     php-apache        cron                                         0s
example-scaler                     php-apache        cron                                         0s
example-scaler                     php-apache        cron                        Unknown   Unknown   0s
example-scaler                     php-apache        cron                        Unknown   Unknown   0s
example-scaler   apps/v1.Deployment   php-apache        cron                        Unknown   Unknown   0s
example-scaler   apps/v1.Deployment   php-apache        cron                        Unknown   Unknown   0s
example-scaler   apps/v1.Deployment   php-apache        cron                        True      Unknown   0s
example-scaler   apps/v1.Deployment   php-apache        cron                        True      False     0s
example-scaler   apps/v1.Deployment   php-apache        cron                        True      False     60s
example-scaler   apps/v1.Deployment   php-apache        cron                        True      True      60s
```

```
$ kubectl get deploy php-apache --watch
NAME         READY   UP-TO-DATE   AVAILABLE   AGE
php-apache   3/3     3            3           4m41s
php-apache   3/4     3            3           7m11s
php-apache   3/4     3            3           7m11s
php-apache   3/4     3            3           7m11s
php-apache   3/4     4            3           7m11s
php-apache   4/4     4            4           7m12s
```
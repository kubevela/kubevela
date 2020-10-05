# cron type Autoscaler

- Apply manifest
```
$ kubectl apply -f standard_v1alpha2_autoscaler.yaml

$ kubectl describe autoscaler example-scaler
Name:         example-scaler
Namespace:    default
Labels:       <none>
Annotations:  API Version:  standard.oam.dev/v1alpha1
Kind:         Autoscaler
Metadata:
  Creation Timestamp:  2020-09-29T03:02:53Z
  Generation:          4
  Resource Version:    835591
  Self Link:           /apis/standard.oam.dev/v1alpha1/namespaces/default/autoscalers/example-scaler
  UID:                 531d875f-4f0d-4b6f-94fe-41a7473d666d
Spec:
  Max Replicas:  8
  Min Replicas:  4
  Target Workload:
    API Version:  extensions/v1beta1
    Kind:         Deployment
    Name:         php-apache
  Triggers:
    Condition:
      Target:  85
    Enabled:   true
    Name:      resource-example
    Type:      cpu
Events:        <none>
```

- Monitor HPA instance and target deployment
```
$ kubectl get hpa --watch
NAME             REFERENCE               TARGETS         MINPODS   MAXPODS   REPLICAS   AGE
example-scaler   Deployment/php-apache   <unknown>/85%   2         4         2          10m
example-scaler   Deployment/php-apache   <unknown>/85%   1         2         2          10m
example-scaler   Deployment/php-apache   <unknown>/85%   4         8         2          11m
example-scaler   Deployment/php-apache   <unknown>/85%   4         8         2          11m
example-scaler   Deployment/php-apache   <unknown>/85%   4         8         4          11m
```

```
$ kubectl get deploy php-apache --watch
NAME         READY   UP-TO-DATE   AVAILABLE   AGE
php-apache   2/2     2            2           17h
php-apache   2/4     2            2           17h
php-apache   2/4     2            2           17h
php-apache   2/4     2            2           17h
php-apache   2/4     4            2           17h
php-apache   3/4     4            3           17h
php-apache   4/4     4            4           17h
```
# How to probe application's status

In this section, I'll illustrate by an example how to declare a probe to test if an application is alive.

## Prerequisites

1. You can access a Kubernetes cluster(remotely or locally like `kind` or `minikube`)
2. You have installed KubeVela

## Steps

### Check application yaml

in [app-with-probe.yaml](./app-with-probe.yaml), you'll see a `livenessProbe` field in `properties`, which shows how to test if a component is alive.

`path` is the health check path your web server is exposed and `port` is the port your server is listening to.

In this example, we use `httpGet` method to check the application. It's a common method in web service. Besides the `httpGet` probe method, you can also change it into `exec` or `tcpSocket` method.

### Apply the application

Just apply the file in cluster.
```shell
kubectl apply -f app-with-probe.yaml
```

### Check the status

the application in the cluster is rendered into resources like `pod`.

Try to describe the pod:
```shell
$ kubectl get pod
NAME                        READY   STATUS    RESTARTS   AGE
frontend-86bc89d8f5-xgrnc   1/1     Running   0          18s

$ kubectl describe pod frontend-86bc89d8f5-xgrnc
...
Liveness:     http-get http://:8080/ delay=0s timeout=1s period=10s #success=1 #failure=3
...(other infomation)
```
You'll see the pod is running without any errors. If this component is unusable reported by livenessProbe, it will restart automatically.

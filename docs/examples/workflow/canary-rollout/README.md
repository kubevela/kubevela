# Canary Rollout

## Prerequisite

Your cluster must have installed [istio](https://istio.io/latest/docs/setup/install/)

## Install definitions

```shell
kubectl apply -f ./traffic-trait-def.yaml
kubectl apply -f ./rollout-wf-def.yaml
```

## Canary rollout workflow

### First deployment

Apply this YAML to deploy application.

```shell
kubectl apply -f first-deploy.yaml
```

Use `kubectl port-forward` map gateway port to localhost
```shell
kubectl port-forward -n istio-system service/istio-ingressgateway 9082:80
```

Wait a few minutes, when rollout have finished. Request back-end service by gateway several times.
```shell
curl  -HHost:back-end.example.com  "http://127.0.0.1:9082/"
```

Will always see return page of `httpd` like this.
```shell
<html><body><h1>It works!</h1></body></html>
```

### Canary rollout part of traffic and replicas to new revision
```shell
kubectl apply -f rollout-v2.yaml
```

Request back-end service by gateway several times.
```shell
curl  -HHost:back-end.example.com  "http://127.0.0.1:9082/"
```

This's a 90% chance still see return page of `httpd`, and 10% see return page of `nginx` like this.

```shell
<!DOCTYPE html>
<html>
<head>
<title>Welcome to nginx!</title>
<style>
    body {
        width: 35em;
        margin: 0 auto;
        font-family: Tahoma, Verdana, Arial, sans-serif;
    }
</style>
</head>
<body>
<h1>Welcome to nginx!</h1>
<p>If you see this page, the nginx web server is successfully installed and
```

### Rollout rest traffic and replicas to new revision

```shell
vela workflow resume  rollout-test
```

Wait a few minutes, when rollout have finished. Request back-end service by gateway several times.
```shell
curl  -HHost:back-end.example.com  "http://127.0.0.1:9082/"
```

Will always see return page of `nginx`.






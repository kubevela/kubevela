# Canary Rollout

## Prerequisite

enable istio addon in you cluster
```shell
vela addon enable istio
```

enable label istio injection in `default namespace`

```shell
kubectl label namespace default istio-injection=enabled
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
curl  http://127.0.0.1:9082/server
```

Will always see return page of `httpd` like this.
```shell
Demo: v1
```

### Canary rollout part of traffic and replicas to new revision
```shell
kubectl apply -f rollout-v2.yaml
```

Request back-end service by gateway several times.
```shell
curl http://127.0.0.1:9082/server
```

This's a 90% chance still see return page of `v1`, and 10% see return page of `v2` like this.

```shell
Demo: v2
```

### Rollout rest traffic and replicas to new revision

```shell
vela workflow resume  canary-test
```

Wait a few minutes, when rollout have finished. Request back-end service by gateway several times.
```shell
curl http://127.0.0.1:9082/server
```

Will always see return page of `v2`.






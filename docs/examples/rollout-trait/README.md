# Application use Rollout trait Example

Here is an example of how to use rollout trait with workload type is webservice.

1. create test namespace
```shell
kubectl create ns  rollout-trait-test
```

2. create application with a component and a rollout trait
```shell
kubectl apply -f ./docs/examples/rollout-trait/application.yaml
```

3. modify container cpu to rollout to component v2
```shell
kubectl apply -f ./docs/examples/rollout-trait/app-v2.yaml
```

4. specify component v1 to revert
```shell
kubectl apply -f ./docs/examples/rollout-trait/app-revert.yaml
```

5. modify cpu again and omit targetRevision to rollout to component v3
```shell
kubectl apply -f ./docs/examples/rollout-trait/app-v3.yaml
```

6. modify targetSize as 7 to scale
```shell
kubectl apply -f ./docs/examples/rollout-trait/app-scale.yaml
```

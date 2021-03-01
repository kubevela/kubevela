# Rollout Example
[kubevela.io](https://kubevela.io)

## Install kruise
```shell 
helm install kruise https://github.com/openkruise/kruise/releases/download/v0.7.0/kruise-chart.tgz
kubectl apply -f charts/vela-core/crds
kubectl apply -f charts/vela-core/templates/defwithtemplate
kubectl apply -f docs/examples/rollout/clonesetDefinition.yaml
kubectl apply -f docs/examples/rollout/app-source.yaml
kubectl apply -f docs/examples/rollout/app-source-prep.yaml
kubectl apply -f docs/examples/rollout/app-target.yaml
kubectl apply -f docs/examples/rollout/app-deploy.yaml
```
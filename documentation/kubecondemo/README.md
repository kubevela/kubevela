minikube start
or
kind create cluster


## KubeWatch

kubectl apply -f https://raw.githubusercontent.com/oam-dev/catalog/master/registry/kubewatch.yaml

## crossplane

kubectl create namespace crossplane-system

helm repo add crossplane-master https://charts.crossplane.io/master/
or helm repo update

helm install crossplane  ../crossplane/ --namespace crossplane-system

## Aliyun provider

vela install
kubectl crossplane install provider crossplane/provider-alibaba:v0.3.0

kubectl create secret generic alibaba-creds --from-literal=accessKeyId=xxxxxx --from-literal=accessKeySecret=xxxxxx -n crossplane-system

kubectl apply -f provider.yaml

## provision 

kubectl crossplane install configuration crossplane/getting-started-with-alibaba:master

## App Config

kubectl apply -f def_db.yaml
kubectl apply -f backendworker.yaml
kubectl apply -f ui.yaml

vela comp deploy database -t rds --app myapp


vela comp deploy data-api -t backendworker --image artursouza/rudr-data-api:0.50  --env [{name: "DATABASE_NAME" value: "postgres"}, {name: "DATABASE_HOSTNAME" valueFrom: secretKeyRef: {name: "db-conn" key: "endpoint"}}, {name: "DATABASE_USER" valueFrom: secretKeyRef: {name: "db-conn" key: "username"}}, {name: "DATABASE_PASSWORD" valueFrom: secretKeyRef: {name: "db-conn" key: "password"}}, {name: "DATABASE_PORT" valueFrom: secretKeyRef: {name: "db-conn" key: "port"}}] --port 3009 --app myapp

vela comp deploy flights-api -t backendworker --image sonofjorel/rudr-flights-api:0.49 --port 3003 --env [{name: "data-uri" value: "http://data-api.default.svc.cluster.local:3009/"}] --app myapp

vela comp deploy quakes-api -t backendworker --image sonofjorel/rudr-quakes-api:0.49 --port 3012 --env [{name: "data-uri" value: "http://data-api.default.svc.cluster.local:3009/"}] --app myapp

vela comp deploy weather-api -t backendworker --image sonofjorel/rudr-weather-api:0.49 --port 3015 --env [{name: "data-uri" value: "http://data-api.default.svc.cluster.local:3009/"}] --app myapp

vela comp deploy webui -t ui --image sonofjorel/rudr-web-ui:0.49 --port 8080 --env [{name: "flights-uri" value: "http://flights-api.default.svc.cluster.local:3003/"},{"weather-uri","http://weather-api.default.svc.cluster.local:3015/"},{"quakes-uri","http://quakes-api.default.svc.cluster.local:3012/"}] --app myapp

## From another terminal:

minikube tunnel
// sample output: route: 10.96.0.0/12 -> 192.168.64.10
// get port 
kubectl get svc web-ui
// sample output : NodePort:                 web-ui  30351/TCP
// access through http://192.168.64.10:30351
// refresh data on all tabs, zoom in when opening each map
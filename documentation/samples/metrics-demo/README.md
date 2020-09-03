# Quick start

This example show case how one can use a metricsTrait to add prometheus monitoring capability to any workload that
 emits metrics data.

## Install Vela core
```shell script
kubectl apply -f charts/third_party/cert-manager
kubectl apply -f charts/third_party/prometheus
## install OAM Prometheus instance
kubectl apply  -f config/oam/prometheus-oam.yaml
make docker-build
kubectl create ns vela-system
helm install kube --namespace vela-system charts/vela-core/
```

## Run ApplicationConfiguration
```shell script
kubectl apply -f documentation/samples/metrics-demo/
workloaddefinition.core.oam.dev/deployments.apps created
traitdefinition.core.oam.dev/services created
traitdefinition.core.oam.dev/metricstraits.standard.oam.dev created
component.core.oam.dev/sample-application created
applicationconfiguration.core.oam.dev/sample-application-with-metrics created
```

## Verify that the metrics are collected on prometheus
```shell script
kubectl --namespace oam-monitoring port-forward svc/prometheus-oam  4848
```
Then access the prometheus dashboard via http://localhost:4848

## Verify that the metrics showing up on grafana
```shell script
kubectl --namespace oam-monitoring port-forward service/monitoring-grafana 3000:80
```
Then access the grafana dashboard via http://localhost:3000.  You shall set the data source URL as `http://prometheus-oam:4848`

## Setup Grafana Panel and Alert
```shell script
kubectl apply -f config/samples/application/dashboard/OAM-Workload-Dashboard.yaml
```
How to set up a Grafana dashboard https://grafana.com/docs/grafana/latest/features/dashboard/dashboards/

Import the dashboard stored in config/samples/application

How to set up a Grafana alert https://grafana.com/docs/grafana/latest/alerting/alerts-overview/. One caveat is that
 only one alert is supported for each panel.

How to set up a DingDing robot as the Grafana notification channel https://ding-doc.dingtalk.com/doc#/serverapi2/qf2nxq
# Metrics

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Path** | **string** | the metric path of the service | [default to /metrics]
**Format** | **string** | +format of the metrics, default as prometheus | [default to prometheus]
**Scheme** | **string** |  | [default to http]
**Enabled** | **bool** |  | [default to true]
**Port** | **int32** | the port for metrics, will discovery automatically by default | [default to 0], >=1024 & <=65535
**Selector** | **map[string]string** | the label selector for the pods, will discovery automatically by default | [optional] 

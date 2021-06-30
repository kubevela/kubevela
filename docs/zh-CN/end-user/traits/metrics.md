---
title:  Metrics
---

## 描述

配置你服务的监控指标。

## 规范

列出 `Metrics` trait 的所有配置项。

```yaml
name: my-app-name

services:
  my-service-name:
    ...
    metrics:
      format: "prometheus"
      port: 8080
      path: "/metrics"
      scheme:  "http"
      enabled: true
```

## 属性

名称 | 描述 | 类型 | 是否必须 | 默认值 
------------ | ------------- | ------------- | ------------- | ------------- 
 path | 服务的指标路径 | string | true | /metrics 
 format | 指标的格式，默认为 prometheus | string | true | prometheus 
 scheme | 检索数据的方式，支持 `http` 和 `https` | string | true | http 
 enabled |  | bool | true | true 
 port | 指标的端口，默认自动暴露 | int | true | 0 
 selector | Pods 的 label selector，默认自动暴露 | map[string]string | false |  

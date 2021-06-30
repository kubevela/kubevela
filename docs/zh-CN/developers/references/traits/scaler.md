---
title:  Scaler
---

## 描述

配置你服务的副本数。

## 规范

列出 `Scaler` trait 的所有配置项。

```yaml
name: my-app-name

services:
  my-service-name:
    ...
    scaler:
      replicas: 100
```

## 属性

名称 | 描述 | 类型 | 是否必须 | 默认值 
------------ | ------------- | ------------- | ------------- | ------------- 
 replicas | Workload 的副本数 | int | true | 1 

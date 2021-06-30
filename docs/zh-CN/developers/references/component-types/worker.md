---
title:  Worker
---

## 描述

描述在后台长期运行，可拓展的容器化服务。它们不需要网络端点来接收外部流量。

## 规格

列出 `Worker` 类型 workload 的所有配置项。

```yaml
name: my-app-name

services:
  my-service-name:
    type: worker
    image: oamdev/testapp:v1
    cmd: ["node", "server.js"]
```

## 属性

名称 | 描述 | 类型 | 是否必须 | 默认值 
------------ | ------------- | ------------- | ------------- | ------------- 
 cmd | 容器中运行的命令 | []string | false |  
 image | 你的服务使用的镜像 | string | true |  

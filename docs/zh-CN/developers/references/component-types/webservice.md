---
title:  Webservice
---

## 描述

描述长期运行的，可伸缩的，容器化的服务，这些服务具有稳定的网络接口，可以接收来自客户的外部网络流量。 如果对于 Appfile 中定义的任何服务，workload type 都被跳过，则默认使用“ webservice”类型。

## 规范

列出 `Webservice` workload 类型的所有配置项。

```yaml
name: my-app-name

services:
  my-service-name:
    type: webservice # could be skipped
    image: oamdev/testapp:v1
    cmd: ["node", "server.js"]
    port: 8080
    cpu: "0.1"
    env:
      - name: FOO
        value: bar
      - name: FOO
        valueFrom:
          secretKeyRef:
            name: bar
            key: bar
```

## 属性

名称 | 描述 | 类型 | 是否必须 | 默认值 
------------ | ------------- | ------------- | ------------- | ------------- 
 cmd | 容器中运行的命令	 | []string | false |  
 env | 使用环境变量定义参数 | [[]env](#env) | false |  
 image | 你的服务所使用到的镜像 | string | true |  
 port | 你要将用户流浪发送到哪个端口 | int | true | 80 
 cpu | 用于服务的CPU单元数，例如0.5（0.5 CPU内核），1（1 CPU内核） | string | false |  


### env

名称 | 描述 | 类型 | 是否必须 | 默认值 
------------ | ------------- | ------------- | ------------- | ------------- 
 name | 环境变量名 | string | true |  
 value | 环境变量值 | string | false |  
 valueFrom | 指定此变量值的源 | [valueFrom](#valueFrom) | false |  


#### valueFrom

名称 | 描述 | 类型 | 是否必须 | 默认值 
------------ | ------------- | ------------- | ------------- | ------------- 
 secretKeyRef | 选择一个 pod 命名空间中的 secret 键 | [secretKeyRef](#secretKeyRef) | true |  


##### secretKeyRef

名称 | 描述 | 类型 | 是否必须 | 默认值 
------------ | ------------- | ------------- | ------------- | ------------- 
 name | 要从 pod 的命名空间中选择的 secret 的名字 | string | true |  
 key | 选择的 secret 键。 必须是有效的 secret 键 | string | true |  


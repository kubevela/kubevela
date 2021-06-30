---
title:  Task
---

## 描述

描述运行完成代码或脚本的作业。

## 规范

列出 `Task` 类型 workload 的所有配置项。

```yaml
name: my-app-name

services:
  my-service-name:
    type: task
    image: perl
    count: 10
    cmd: ["perl",  "-Mbignum=bpi", "-wle", "print bpi(2000)"]
```

## 属性

名称 | 描述 | 类型 | 是否必须 | 默认值 
------------ | ------------- | ------------- | ------------- | ------------- 
 cmd | 容器中运行的命令 | []string | false |  
 count | 指定并行运行的 task 数量 | int | true | 1 
 restart | 定义作业重启策略，值只能为 Never 或 OnFailure。 | string | true | Never 
 image | 你的服务使用的镜像 | string | true |  

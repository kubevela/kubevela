---
title:  Workload2
---

## 描述

。

## 参数说明


 名称 | 描述 | 类型 | 是否必须 | 默认值 | 不可变 
 ------ | ------ | ------ | ------------ | --------- | --------- 
 acl | OSS bucket ACL, supported 'private', 'public-read', 'public-read-write'。 | string | false |  |  
 bucket | OSS bucket name。 | string | false |  |  
 writeConnectionSecretToRef | The secret which the cloud resource connection will be written to。 | [writeConnectionSecretToRef](#writeConnectionSecretToRef) | false |  |  


#### writeConnectionSecretToRef

 名称 | 描述 | 类型 | 是否必须 | 默认值 | 不可变 
 ------ | ------ | ------ | ------------ | --------- | --------- 
 name | The secret name which the cloud resource connection will be written to。 | string | true |  |  
 namespace | The secret namespace which the cloud resource connection will be written to。 | string | false |  |  


### 输出

WriteConnectionSecretToRefIntroduction

 名称 | 描述 
 ------------ | ------------- 
 BUCKET_NAME | 

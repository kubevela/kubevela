---
title:  Task
---

## Description

Describes jobs that run code or a script to completion.

## Specification

List of all configuration options for a `Task` workload type.

```yaml
...
    image: perl
    count: 10
    cmd: ["perl",  "-Mbignum=bpi", "-wle", "print bpi(2000)"]
```

## Properties

Name | Description | Type | Required | Default 
------------ | ------------- | ------------- | ------------- | ------------- 
 cmd | Commands to run in the container | []string | false |  
 count | specify number of tasks to run in parallel | int | true | 1 
 restart | Define the job restart policy, the value can only be Never or OnFailure. By default, it's Never. | string | true | Never 
 image | Which image would you like to use for your service | string | true |  

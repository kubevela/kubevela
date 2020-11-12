# Task

## Description

`Task` is a workload type to describe jobs that run code or a script to completion.

## Specification

List of all configuration options for a `Task` workload type.

```yaml
name: my-app-name

services:
  my-service-name:
    type: task
    image: perl
    count: 10
    cmd: ["perl",  "-Mbignum=bpi", "-wle", "print bpi(2000)"]
```


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Cmd** | **[]string** |  | [optional] 
**Count** | **int32** | specify number of tasks to run in parallel | [default to 1]
**Image** | **string** | Which image would you like to use for your service | 

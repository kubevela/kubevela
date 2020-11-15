## Description

`Autoscale` is used to automatically scale workloads by resource utilization metrics and cron

## Specification

List of all available properties for a `Autoscale` trait.

```yaml
name: testapp

services:
express-server:

  autoscale:
    min: 1
    max: 4
    cron:
      startAt:  "14:00"
      duration: "2h"
      days:     "Monday, Thursday"
      replicas: 2
      timezone: "America/Los_Angeles"
    cpuPercent: 10
```

## Properties

Name | Type  | Description | Notes
------------  | ------------- | ------------- | ------------- 
 min | int |  minimal replicas of the workload | required 
 max | int |  maximal replicas of the workload | required 
 cpuPercent | int |  specify the value for CPU utilization, like 80, which means 80% |  
 cron | [{Cron}](#Cron) |  just for `appfile`, not available for Cli usage |  

### Cron

Name | Type |  Description | Notes
------------ | ------------- | ------------- | ------------- 
 startAt | string |  the time to start scaling, like `08:00` |  
 duration | string |  for how long the scaling will last |  
 days | string |  several workdays or weekends, like "Monday, Tuesday" |  
 replicas | int |  the target replicas to be scaled to |  
 timezone | string |  timezone, like "America/Los_Angeles" |  


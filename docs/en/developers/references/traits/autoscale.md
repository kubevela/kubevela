---
title:  Autoscale
---

## Description

Automatically scales workloads by resource utilization metrics or cron triggers.

## Specification

List of all configuration options for a `Autoscale` trait.

```yaml
...
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

Name | Description | Type | Required | Default 
------------ | ------------- | ------------- | ------------- | ------------- 
 min | Minimal replicas of the workload | int | true |  
 max | Maximal replicas of the workload | int | true |  
 cpuPercent | Specify the value for CPU utilization, like 80, which means 80% | int | false |  
 cron | Cron type auto-scaling. Just for `appfile`, not available for Cli usage | [cron](#cron) | false |  


### cron

Name | Description | Type | Required | Default 
------------ | ------------- | ------------- | ------------- | ------------- 
 startAt | The time to start scaling, like `08:00` | string | true |  
 duration | For how long the scaling will last | string | true |  
 days | Several workdays or weekends, like "Monday, Tuesday" | string | true |  
 replicas | The target replicas to be scaled to | int | true |  
 timezone | Timezone, like "America/Los_Angeles" | string | true |  

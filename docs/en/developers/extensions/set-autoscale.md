---
title:  Automatically scale workloads by resource utilization metrics and cron
---



## Prerequisite
Make sure auto-scaler trait controller is installed in your cluster

Install auto-scaler trait controller with helm

1. Add helm chart repo for autoscaler trait
    ```shell script
    helm repo add oam.catalog  http://oam.dev/catalog/
    ```

2. Update the chart repo
    ```shell script
    helm repo update
    ```

3. Install autoscaler trait  controller
    ```shell script
    helm install --create-namespace -n vela-system autoscalertrait oam.catalog/autoscalertrait

Autoscale depends on metrics server, please [enable it in your Kubernetes cluster](../references/devex/faq#autoscale-how-to-enable-metrics-server-in-various-kubernetes-clusters) at the beginning.

> Note: autoscale is one of the extension capabilities [installed from cap center](../cap-center),
> please install it if you can't find it in `vela traits`.

## Setting cron auto-scaling policy
Introduce how to automatically scale workloads by cron.

1. Prepare Appfile

  ```yaml
  name: testapp
  
  services:
    express-server:
      # this image will be used in both build and deploy steps
      image: oamdev/testapp:v1
  
      cmd: ["node", "server.js"]
      port: 8080
  
      autoscale:
        min: 1
        max: 4
        cron:
          startAt:  "14:00"
          duration: "2h"
          days:     "Monday, Thursday"
          replicas: 2
          timezone: "America/Los_Angeles"
  ```

> The full specification of `autoscale` could show up by `$ vela show autoscale` or be found on [its reference documentation](../references/traits/autoscale)

2. Deploy an application
  
  ```
  $ vela up
    Parsing vela.yaml ...
    Loading templates ...
    
    Rendering configs for service (express-server)...
    Writing deploy config to (.vela/deploy.yaml)
    
    Applying deploy configs ...
    Checking if app has been deployed...
    App has not been deployed, creating a new deployment...
    ✅ App has been deployed 🚀🚀🚀
        Port forward: vela port-forward testapp
                 SSH: vela exec testapp
             Logging: vela logs testapp
          App status: vela status testapp
      Service status: vela status testapp --svc express-server
  ```

3. Check the replicas and wait for the scaling to take effect

  Check the replicas of the application, there is one replica.

  ```
  $ vela status testapp
  About:
  
    Name:      	testapp
    Namespace: 	default
    Created at:	2020-11-05 17:09:02.426632 +0800 CST
    Updated at:	2020-11-05 17:09:02.426632 +0800 CST
  
  Services:
  
    - Name: express-server
      Type: webservice
      HEALTHY Ready: 1/1
      Traits:
        - ✅ autoscale: type: cron    replicas(min/max/current): 1/4/1
      Last Deployment:
        Created at: 2020-11-05 17:09:03 +0800 CST
        Updated at: 2020-11-05T17:09:02+08:00
  ```
  
  Wait till the time clocks `startAt`, and check again. The replicas become to two, which is specified as 
  `replicas` in `vela.yaml`.
  
  ```
  $ vela status testapp
  About:
  
    Name:      	testapp
    Namespace: 	default
    Created at:	2020-11-10 10:18:59.498079 +0800 CST
    Updated at:	2020-11-10 10:18:59.49808 +0800 CST
  
  Services:
  
    - Name: express-server
      Type: webservice
      HEALTHY Ready: 2/2
      Traits:
        - ✅ autoscale: type: cron    replicas(min/max/current): 1/4/2
      Last Deployment:
        Created at: 2020-11-10 10:18:59 +0800 CST
        Updated at: 2020-11-10T10:18:59+08:00
  ```
  
  Wait after the period ends, the replicas will be one eventually.

## Setting auto-scaling policy of CPU resource utilization
Introduce how to automatically scale workloads by CPU resource utilization.

1. Prepare Appfile

  Modify `vela.yaml` as below. We add field `services.express-server.cpu` and change the auto-scaling policy
  from cron to cpu utilization by updating filed `services.express-server.autoscale`.
  
  ```yaml
  name: testapp
  
  services:
    express-server:
      image: oamdev/testapp:v1
        
      cmd: ["node", "server.js"]
      port: 8080
      cpu: "0.01"
  
      autoscale:
        min: 1
        max: 5
        cpuPercent: 10
  ```

2. Deploy an application

  ```bash
  $ vela up
  ```

3. Expose the service entrypoint of the application

  ```
  $ vela port-forward helloworld 80
  Forwarding from 127.0.0.1:80 -> 80
  Forwarding from [::1]:80 -> 80

  Forward successfully! Opening browser ...
  Handling connection for 80
  Handling connection for 80
  Handling connection for 80
  Handling connection for 80
  ```

  On your macOS, you might need to add `sudo` ahead of the command.

4. Monitor the replicas changing

  Continue to monitor the replicas changing when the application becomes overloaded. You can use Apache HTTP server
  benchmarking tool `ab` to mock many requests to the application.

  ```
  $ ab -n 10000 -c 200 http://127.0.0.1/
  This is ApacheBench, Version 2.3 <$Revision: 1843412 $>
  Copyright 1996 Adam Twiss, Zeus Technology Ltd, http://www.zeustech.net/
  Licensed to The Apache Software Foundation, http://www.apache.org/

  Benchmarking 127.0.0.1 (be patient)
  Completed 1000 requests
  ```

  The replicas gradually increase from one to four.

  ```
  $ vela status helloworld --svc frontend
  About:

    Name:      	helloworld
    Namespace: 	default
    Created at:	2020-11-05 20:07:21.830118 +0800 CST
    Updated at:	2020-11-05 20:50:42.664725 +0800 CST

  Services:

    - Name: frontend
      Type: webservice
      HEALTHY Ready: 1/1
      Traits:
        - ✅ autoscale: type: cpu     cpu-utilization(target/current): 5%/10%	replicas(min/max/current): 1/5/2
      Last Deployment:
        Created at: 2020-11-05 20:07:23 +0800 CST
        Updated at: 2020-11-05T20:50:42+08:00
  ```

  ```
  $ vela status helloworld --svc frontend
  About:

    Name:      	helloworld
    Namespace: 	default
    Created at:	2020-11-05 20:07:21.830118 +0800 CST
    Updated at:	2020-11-05 20:50:42.664725 +0800 CST

  Services:

    - Name: frontend
      Type: webservice
      HEALTHY Ready: 1/1
      Traits:
        - ✅ autoscale: type: cpu     cpu-utilization(target/current): 5%/14%	replicas(min/max/current): 1/5/4
      Last Deployment:
        Created at: 2020-11-05 20:07:23 +0800 CST
        Updated at: 2020-11-05T20:50:42+08:00
  ```

  Stop `ab` tool, and the replicas will decrease to one eventually.

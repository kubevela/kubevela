# Automatically scale workloads by resource utilization metrics and cron

Contents:
- [Scale by CPU resource utilization metrics](#Scale by CPU resource utilization metrics)
- [Scale workload by cron](#Scale workload by cron)

## Scale by CPU resource utilization metrics
Introduce how to automatically scale workloads by resource utilization metrics in Cli. Currently, only cpu utilization 
is supported.
   
- Deploy an application

  Run the following command to deploy application `helloworld`.
  
  ```
  $ vela svc deploy frontend -t webservice -a helloworld --image nginx:1.9.2 --port 80 --cpu-requests=0.05
  App helloworld deployed
  ```
  
  By default, the replicas of the workload webservice `helloworld` is.

- Scale the application by CPU utilization metrics
  ```
  $ vela autoscale helloworld --svc frontend --min 1 --max 5 --cpu 5
  Adding autoscale for app frontend
  ⠋ Checking Status ...
  ✅ Application Deployed Successfully!
    - Name: frontend
      Type: webservice
      HEALTHY Ready: 1/1
      Traits:
        - ✅ autoscale: type: cpu	minReplicas: 1	maxReplicas: 5	CPUUtilization(target/current): 5%/0%	replicas: 0
      Last Deployment:
        Created at: 2020-11-06 16:10:54 +0800 CST
        Updated at: 2020-11-06T16:19:04+08:0
  ```
  
- Access the application with heavy requests
  
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
  
- Use Apache HTTP server benchmarking tool `ab` to access the application.
  
  ```
  $ ab -n 10000 -c 200 http://127.0.0.1/
  This is ApacheBench, Version 2.3 <$Revision: 1843412 $>
  Copyright 1996 Adam Twiss, Zeus Technology Ltd, http://www.zeustech.net/
  Licensed to The Apache Software Foundation, http://www.apache.org/
  
  Benchmarking 127.0.0.1 (be patient)
  Completed 1000 requests
  ```
  
  Monitor the replicas of the workload, and its replicas gradually increase from one to four. 
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
        - ✅ autoscale: type: cpu	minReplicas: 1	maxReplicas: 5	CPUUtilization(target/current): 5%/10%	replicas: 2
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
        - ✅ autoscale: type: cpu	minReplicas: 1	maxReplicas: 5	CPUUtilization(target/current): 5%/14%	replicas: 4
      Last Deployment:
        Created at: 2020-11-05 20:07:23 +0800 CST
        Updated at: 2020-11-05T20:50:42+08:00
  ```
  
  Stop `ab` tool, and the replicas will decrease to one eventually.


## Scale workload by cron
Introduce how to automatically scale workloads by cron in Appfile.

- Prepare Appfile
  Follow the instructions of [appfile](./devex/appfile.md) to prepare the `vela.yaml` as below.
  ```yaml
  name: testapp
  
  services:
    express-server:
      # this image will be used in both build and deploy steps
      image: zzxwill/kubevela-appfile-demo:v1
  
      build:
        # Here more runtime specific build templates will be supported, like NodeJS, Go, Python, Ruby.
        docker:
          file: Dockerfile
          context: .
  
      cmd: ["node", "server.js"]
      port: 8080
  
      autoscale:
        minReplicas: 1
        maxReplicas: 4
        cron:
          startAt:  "14:00"
          duration: "2h"
          days:     "Monday, Thursday"
          replicas: "2"
          timezone: "America/Seattle"
  ```

- Deploy an application
  Run the following command to deploy the application defined in `vela.yaml`.
  
  ```
  $ vela up
  Parsing vela.yaml ...
  Loading templates ...
  
  Building service (express-server)...
  #2 [internal] load build definition from Dockerfile
  #2 sha256:c25a03ff9861be1da16a316055d11b83778efa23c655d0e69a902487bbf3c303
  #2 transferring dockerfile: 37B 0.0s done
  #2 DONE 0.1s
  
  ...
  
  pushing image (zzxwill/kubevela-appfile-demo:v1)...
  The push refers to repository [docker.io/zzxwill/kubevela-appfile-demo]
  1893e9ad9204: Preparing
  b60a6f0fd043: Preparing
  ...
  89ae5c4ee501: Layer already exists
  b60a6f0fd043: Layer already exists
  1893e9ad9204: Pushed
  v1: digest: sha256:11e48ce2205a1d92c1c920b3a3f41d3ee357fa2794261dc0d0e8010068e68da6 size: 1365
  
  
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

- Check the replicas and wait for the scaling to take effect

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
        - ✅ autoscale: type: cron	minReplicas: 1	maxReplicas: 4	replicas: 1
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
    Created at:	2020-11-05 17:09:02.426632 +0800 CST
    Updated at:	2020-11-05 17:09:02.426632 +0800 CST
  
  Services:
  
    - Name: express-server
      Type: webservice
      HEALTHY Ready: 1/1
      Traits:
        - ✅ autoscale: type: cron	minReplicas: 1	maxReplicas: 4	replicas: 2
      Last Deployment:
        Created at: 2020-11-05 17:09:03 +0800 CST
        Updated at: 2020-11-05T17:09:02+08:00
  ```
  
  Wait after the period ends, the replicas will be one eventually.

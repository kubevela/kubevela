# Automatically scale workloads by resource utilization metrics and cron

## Scale by CPU resource utilization metrics

1. Prepare Appfile:

  ```bash
    $ cat <<EOF > vela.yaml
      name: testapp
      
      services:
        express-server:
          image: nginx:1.9.2
      
          port: 80
          cpuRequests: 0.05
      
          autoscale:
            min: 1
            max: 5
            cpu: 5
    EOF
  ```

2. Deploy the application:
  
  ```bash
    $ vela up
  ```
  
3. Access the application with heavy requests
  
  ```
    $ vela port-forward helloworld 80
    Forwarding from 127.0.0.1:80 -> 80
    Forwarding from [::1]:80 -> 80
    
    Forward successfully! Opening browser ...
    Handling connection for 80
  ```
  
  On your macOS, you might need to add `sudo` ahead of the command.
  
4. Use Apache HTTP server benchmarking tool `ab` to access the application.
  
  ```
    $ ab -n 10000 -c 200 http://127.0.0.1/
    This is ApacheBench, Version 2.3 <$Revision: 1843412 $>
    Copyright 1996 Adam Twiss, Zeus Technology Ltd, http://www.zeustech.net/
    Licensed to The Apache Software Foundation, http://www.apache.org/
    
    Benchmarking 127.0.0.1 (be patient)
    Completed 1000 requests
  ```
  
5. Monitor the replicas of the workload, and see its replicas increase from one to four.

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


## Scale workload by cron

1. Prepare Appfile:

  ```bash
    $ cat <<EOF > vela.yaml
      name: testapp
      
      services:
        express-server:
          image: zzxwill/kubevela-appfile-demo:v1
      
          cmd: ["node", "server.js"]
          port: 8080
      
          autoscale:
            min: 1
            max: 4
            cron:
              startAt:  "14:00"
              duration: "2h"
              days:     "Monday, Thursday"
              replicas: "2"
              timezone: "America/Seattle"
    EOF
  ```

2. Deploy the application:
  
  ```bash
    $ vela up
  ```

3. Check the replicas and wait for the scaling to take effect:

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

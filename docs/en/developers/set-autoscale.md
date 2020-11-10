# Automatically scale workload by cron or resource utilization

## Prerequisites
 - [ ] [KEDA v2.0 Beta](https://keda.sh/blog/keda-2.0-beta/)
 
   KEDA will be automatically deployed during vela installation, so just run the following command.
   ```shell
   $ vela install
   ```
   
## Scale an application
   
- Deploy an application

  Run the following command to deploy application `helloworld`.
  
  ```
  $ vela svc deploy frontend -t webservice -a helloworld --image nginx:1.9.2 --port 80
  App helloworld deployed
  ```
  
  Check the replicas of Deployment `frontend` which is deployed by workload webservice `helloworld` and there is one replica.
  
  (TODO: The command below needs to be replaced  with `vela show` to check the replicas.)
  ```
  $ kubectl get deploy frontend
  NAME       READY   UP-TO-DATE   AVAILABLE   AGE
  frontend   1/1     1            1           2m52s
  ```

- Scale the application by `cron`
  ```
  $ vela autoscale helloworld --svc frontend --minReplicas 1 --maxReplicas 4 --replicas 2 --name cron-test --startAt 21:00 --duration 2h --days "Monday, Tuesday"
  Adding autoscale for app frontend
  ⠋ Deploying ...
  ✅ Application Deployed Successfully!
    - Name: frontend
      Type: webservice
      HEALTHY Ready: 1/1
      Last Deployment:
        Created at: 2020-11-03 20:53:50 +0800 CST
        Updated at: 2020-11-03T21:01:20+08:00
      Traits:
        - autoscale: 
            maxReplicas=4
            minReplicas=1
            replicas=2
            startAt=21:00
            timezone=Asia/Shanghai
            days=Monday, Tuesday
            name=cron-test
            type=cron
            duration=2h
  ```
  
  The time is `21:07` which is in the active period of the trait which started at `21:00` and the duration is two hours.
  Check the replicas of Deployment `frontend` again, it has been scaled to 2.
  ```
  $ kubectl get deploy
  NAME       READY   UP-TO-DATE   AVAILABLE   AGE
  frontend   2/2     2            2           8m42s
  ```
  
  Wait after the period ends, the replicas will be one eventually.


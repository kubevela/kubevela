# Alternative Commands

Besides Appfile, KubeVela also provides a set of alternatives commands to deploy the application. Think about "shortcuts" that could generate and apply Appfile without the need to write YAML file manually.

> NOTE: These shortcuts are based on Appfile and designed for quick demo purpose only, we would recommend using Appfile instead for serious usage of KubeVela.

## `vela init`

A shortcut to initialize and deploy an application with one service, run:

> If you only want to initialize the Appfile only (i.e. dry run), add `--render-only` flag

```bash
$ vela init
Welcome to use KubeVela CLI! We're going to help you run applications through a couple of questions.

Environment: default, namespace: default

? What is the domain of your application service (optional):  example.com
? What is your email (optional, used to generate certification):
? What would you like to name your application (required):  testapp
? Choose the workload type for your application (required, e.g., webservice):  webservice
? What would you like to name this webservice (required):  testsvc
? Which image would you like to use for your service (required): crccheck/hello-world
? Which port do you want customer traffic sent to (optional, default is 80): 8000

...
✅ Application Deployed Successfully!
  - Name: testsvc
    Type: webservice
    HEALTHY Ready: 1/1
    Routes:

    Last Deployment:
      Created at: ...
      Updated at: ...
```

Check the application:

```bash
$ vela show testapp
About:

  Name:      	testapp
  Created at:	...
  Updated at:	...


Environment:

  Namespace:	default

Services:

  - Name:        	testsvc
    WorkloadType:	webservice
    Arguments:
      image:        	crccheck/hello-world
      port:         	8000
      Traits:
```

## `vela svc deploy`

A shortcut to initialize and deploy service one by one.

Firstly, check the available workload types.

```bash
$ vela workloads
NAME      	DESCRIPTION
worker   	Backend worker without ports exposed
webservice	Long running service with network routes
```

Deploy the first service named `frontend` with `Web Service` type:

```bash
$ vela svc deploy frontend --app testapp -t webservice --image crccheck/hello-world
App testapp deployed
```

Deploy the second service named `backend` with "Backend Worker" type:

```bash
$ vela svc deploy backend --app testapp2 -t worker --image crccheck/hello-world
App testapp2 deployed
```

```bash
$ vela ls
SERVICE 	APP     	TYPE	TRAITS	STATUS 	CREATED-TIME
frontend	testapp 	    	      	...
backend 	testapp 	    	      	...
```

## `vela route`

A shortcut to add route config.

```bash
$ vela route testapp --domain frontend.mycustom.domain
Adding route for app frontend

Rendering configs for service (frontend)...
⠋ Deploying ...
✅ Application Deployed Successfully!
Showing status of service(type: webservice) frontend deployed in Environment myenv
Service frontend Status:   HEALTHY Ready: 1/1
  route:  Visiting URL: http://frontend.mycustom.domain IP: 123.57.10.233

Last Deployment:
  Created at: 2020-10-29 15:45:13 +0800 CST
  Updated at: 2020-10-29T16:12:45+08:00
```

Then you will be able to visit by:

```shell script
$ curl -H "Host:frontend.mycustom.domain" 123.57.10.233
```

If you have domain set in deployment environment

```bash
$ vela route testapp
Adding route for app frontend

Rendering configs for service (frontend)...
⠋ Deploying ...
✅ Application Deployed Successfully!
Showing status of service(type: webservice) frontend deployed in Environment default
Service frontend Status:   HEALTHY Ready: 1/1
  route:  Visiting URL: https://frontend.123.57.10.233.xip.io IP: 123.57.10.233

Last Deployment:
  Created at: 2020-10-29 11:26:46 +0800 CST
  Updated at: 2020-10-29T11:28:01+08:00
```

## `vela autoscale`

A shortcut to autoscale the service.
Currently, Cli only supports setting CPU resource utilization auto-scaling policy. To configure cron auto-scaling policy,
please refer to [autoscale in Appfile](/en/developers/set-autoscale.md).

- Deploy an application

  Run the following command to deploy application `helloworld`.

  ```
  $ vela svc deploy frontend -t webservice -a helloworld --image nginx:1.9.2 --port 80 --cpu=0.05
  App helloworld deployed
  ```

  By default, the replicas of the workload webservice `helloworld` is one.

- Scale the application by CPU utilization metrics
  ```
  $ vela autoscale helloworld --svc frontend --min 1 --max 5 --cpu-percent 5
  Adding autoscale for app frontend
  ⠋ Checking Status ...
  ✅ Application Deployed Successfully!
    - Name: frontend
      Type: webservice
      HEALTHY Ready: 1/1
      Traits:
        - ✅ autoscale: type: cpu     cpu-utilization(target/current): 5%/0%	replicas(min/max/current): 1/5/0
      Last Deployment:
        Created at: 2020-11-06 16:10:54 +0800 CST
        Updated at: 2020-11-06T16:19:04+08:0
  ```

- Monitor the replicas changing when the application becomes overloaded

  Continue to monitor the replicas changing when the application becoming overloaded. You can use Apache HTTP server
  benchmarking tool `ab` to mock many requests to the application as we did in [Autoscalig in Appfile](/en/developers/set-autoscale.md).

  With more and more requests to the application, the replicas gradually increase from one to four.

## `vela metric`

A shortcut to add metrics config.

If your application has exposed metrics, you can easily setup monitoring system
with the help of `metric` capability.

Let's run [`christianhxc/gorandom:1.0`](https://github.com/christianhxc/prometheus-tutorial) as an example app.
The app will emit random latencies as metrics.

```bash
$ vela svc deploy metricapp -t webservice --image christianhxc/gorandom:1.0 --port 8080
```

Then add metric by:

```bash
$ vela metric metricapp
Adding metric for app metricapp
⠋ Deploying ...
✅ Application Deployed Successfully!
  - Name: metricapp
    Type: webservice
    HEALTHY Ready: 1/1
    Routes:
      - ✅ metric: Monitoring port: 8080, path: /metrics, format: prometheus, schema: http.
    Last Deployment:
      Created at: 2020-11-02 14:31:56 +0800 CST
      Updated at: 2020-11-02T14:32:00+08:00
```

The metrics trait will automatically discover port and label to monitor if no parameters specified.
If more than one ports found, it will choose the first one by default.

Verify that the metrics are collected on prometheus
<details>

```shell script
$ kubectl --namespace monitoring port-forward `k -n monitoring get pods -l prometheus=oam -o name` 9090
```

Then access the prometheus dashboard via http://localhost:9090/targets

</details>

## `vela rollout`

A shortcut to add rollout config.

Firstly, deploy your app by:

```shell script
$ vela svc deploy testapp -t webservice --image oamdev/testapp:v1 --port 8080
App testapp deployed
```

Add route for visit:

```shell script
$ vela route testapp --domain myhost.com
Adding route for app testapp
⠋ Checking Status ...
✅ Application Deployed Successfully!
  - Name: testapp
    Type: webservice
    HEALTHY Ready: 1/1
    Traits:
      - ✅ route:  Visiting URL: http://myhost.com IP: <your-ingress-IP-address>

    Last Deployment:
      Created at: 2020-11-09 12:50:30 +0800 CST
      Updated at: 2020-11-09T12:51:19+08:00
```

```shell script
$ curl -H "Host:myhost.com" http://<your-ingress-IP-address>/
Hello World%
```

Secondly, add rollout policy for your app:

```shell script
$ vela rollout testapp --replica 5 --step-weight 20 --interval 5s
```

Then update your app by:

```shell script
$ vela svc deploy testapp -t webservice --image oamdev/testapp:v2 --port 8080
```

Then it will rolling update your instance, you could try `curl` your app multiple times:

```shell script
$ curl -H "Host:myhost.com" http://39.97.232.19/
Hello World  -- Updated Version Two!%                                         
$ curl -H "Host:myhost.com" http://39.97.232.19/
Hello World%                                                                  
$ curl -H "Host:myhost.com" http://39.97.232.19/
Hello World%                                                                  
$ curl -H "Host:myhost.com" http://39.97.232.19/
Hello World  -- Updated Version Two!%                                         
$ curl -H "Host:myhost.com" http://39.97.232.19/
Hello World%                                                                  
$ curl -H "Host:myhost.com" http://39.97.232.19/
Hello World  -- Updated Version Two!%
``` 

It will return both version of output info as both instances are all existing.

<details>
  <summary>Under the hood, it was flagger canary running.</summary>

```shell script
$ kubectl get canaries.flagger.app testapp-trait-76fc76fddc -w
NAME                       STATUS        WEIGHT   LASTTRANSITIONTIME
testapp-trait-76fc76fddc   Progressing   0        2020-11-10T09:06:10Z
testapp-trait-76fc76fddc   Progressing   20       2020-11-10T09:06:30Z
testapp-trait-76fc76fddc   Progressing   40       2020-11-10T09:06:40Z
testapp-trait-76fc76fddc   Progressing   60       2020-11-10T09:07:31Z
testapp-trait-76fc76fddc   Promoting     0        2020-11-10T09:08:00Z
testapp-trait-76fc76fddc   Promoting     100      2020-11-10T09:08:10Z
testapp-trait-76fc76fddc   Finalising    0        2020-11-10T09:08:20Z
testapp-trait-76fc76fddc   Succeeded     0        2020-11-10T09:08:30Z
```
</details>

# Setting Rollout Strategy

You could use rollout capability to rolling upgrade your app.

The workflow will like below:

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
      - ✅ route: 	Visiting URL: http://myhost.com	IP: <your-ingress-IP-address>

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
vela rollout testapp --replica 5 --step-weight 20 --interval 5s
```

Then update your app by:

```shell script
vela svc deploy testapp -t webservice --image oamdev/testapp:v2 --port 8080
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
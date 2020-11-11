# Setting Rollout Strategy

The `rollout` section is used to configure rolling update policy for your app.

Add rollout config under `express-server` along with the [`route` config](./set-rollout.md).

As for convenience， the complete example would like as below:

```yaml
name: testapp
services:
  express-server:
    type: webservice
    image: oamdev/testapp:v1
    port: 8080
    rollout:
      replica: 5
      stepWeight: 20
      interval: "30s"
    route:
      domain: example.com
```

Apply this `appfile.yaml`:

```bash
$ vela up
```


You could use rollout capability to 

The workflow will like below:

Firstly, deploy your app by:

```bash
$ vela svc deploy testapp -t webservice --image oamdev/testapp:v1 --port 8080
App testapp deployed
```

You could check the status by:

```bash
$ vela status testapp
About:

  Name:      	testapp
  Namespace: 	myenv
  Created at:	2020-11-09 17:34:38.064006 +0800 CST
  Updated at:	2020-11-10 17:05:53.903168 +0800 CST

Services:

  - Name: testapp
    Type: webservice
    HEALTHY Ready: 5/5
    Traits:
      - ✅ rollout: interval=5s
		replica=5
		stepWeight=20
      - ✅ route: 	Visiting URL: http://example.com	IP: <your-ingress-IP-address>

    Last Deployment:
      Created at: 2020-11-09 17:34:38 +0800 CST
      Updated at: 2020-11-10T17:05:53+08:00
```

Visiting this app by:

```bash
$ curl -H "Host:example.com" http://<your-ingress-IP-address>/
Hello World%
```

In day 2, assuming we have make some changes on our app and build the new image and name it by `oamdev/testapp:v2`.

Let's update the appfile by:

```yaml
name: testapp
services:
  express-server:
    type: webservice
-   image: oamdev/testapp:v1
+   image: oamdev/testapp:v2
    port: 8080
    rollout:
      replica: 5
      stepWeight: 20
      interval: "30s"
    route:
      domain: example.com
```

Apply this `appfile.yaml` again:

```bash
$ vela up
```

Then it will rolling update your instance, you could try `curl` your app multiple times:

```bash
$ curl -H "Host:example.com" http://<your-ingress-ip-address>/
Hello World  -- Updated Version Two!%                                         
$ curl -H "Host:example.com" http://<your-ingress-ip-address>/
Hello World%                                                                  
$ curl -H "Host:example.com" http://<your-ingress-ip-address>/
Hello World%                                                                  
$ curl -H "Host:example.com" http://<your-ingress-ip-address>/
Hello World  -- Updated Version Two!%                                         
$ curl -H "Host:example.com" http://<your-ingress-ip-address>/
Hello World%                                                                  
$ curl -H "Host:example.com" http://<your-ingress-ip-address>/
Hello World  -- Updated Version Two!%
``` 

It will return both version of output info as both instances are all existing.

<details>
  <summary>Under the hood, it was flagger canary running.</summary>

```bash
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
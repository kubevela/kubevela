# Setting Rollout Strategy

The `rollout` section is used to configure rolling update policy for your app.

Add rollout config under `express-server` along with a `route`.

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

> The full specification of `rollout` could be found [here](references/traits/rollout.md)

Apply this `appfile.yaml`:

```bash
$ vela up
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

You could then try to `curl` your app multiple times and and see how the new instances being promoted following Canary rollout strategy:

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

For every 30 second, 20% more traffic will be shifted to the new instance from the old instance as we configured in Appfile.


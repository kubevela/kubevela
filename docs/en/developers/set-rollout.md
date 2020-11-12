# Setting Rollout Strategy

The `rollout` section is used to configure rolling update policy for your app.

Add rollout config under `express-server` along with a `route`.

```yaml
name: testapp
services:
  express-server:
    type: webservice
    image: oamdev/testapp:rolling01
    port: 80

    rollout:
      replica: 5
      stepWeight: 20
      interval: "30s"
    
    route:
      domain: "example.com"
```

If your cluster don't have ingress, you could set domain to be empty like below:

```yaml
...
    route:
-     domain: "example.com"
+     domain: ""
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
Hello World -- Rolling 01
```

If you don't have ingress in your cluster and you leave domain to be empty, then you could visit this app by:

```bash
$ vela port-forward testapp --route
Forwarding from 127.0.0.1:8080 -> 80
Forwarding from [::1]:8080 -> 80

Forward successfully! Opening browser ...
Handling connection for 8080
```

It will automatically open browser for you.

In day 2, assuming we have make some changes on our app and build the new image and name it by `oamdev/testapp:v2`.

Let's update the appfile by:

```yaml
name: testapp
services:
  express-server:
    type: webservice
-   image: oamdev/testapp:rolling01
+   image: oamdev/testapp:rolling02
    port: 80
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


You could run `vela status` several times to see the instance rolling:

```shell script
$ vela status testapp
About:

  Name:      	testapp
  Namespace: 	myenv
  Created at:	2020-11-12 19:02:40.353693 +0800 CST
  Updated at:	2020-11-12 19:02:40.353693 +0800 CST

Services:

  - Name: express-server
    Type: webservice
    HEALTHY express-server-v2:Ready: 1/1 express-server-v1:Ready: 4/4
    Traits:
      - ✅ rollout: interval=30s
		replica=5
		stepWeight=20
      - ✅ route: 	 Visiting by using 'vela port-forward testapp --route'

    Last Deployment:
      Created at: 2020-11-12 17:20:46 +0800 CST
      Updated at: 2020-11-12T19:02:40+08:00
```

You could then try to `curl` your app multiple times and and see how the new instances being promoted following Canary
rollout strategy: (Note: Using `vela port-forward` will not see this as port-forward will proxy network on fixed port, only ingress
has loadbalance.)

```bash
$ curl -H "Host:example.com" http://<your-ingress-ip-address>/
Hello World -- This is rolling 02                                        
$ curl -H "Host:example.com" http://<your-ingress-ip-address>/
Hello World -- Rolling 01                                                                
$ curl -H "Host:example.com" http://<your-ingress-ip-address>/
Hello World -- Rolling 01                                                    
$ curl -H "Host:example.com" http://<your-ingress-ip-address>/
Hello World -- This is rolling 02                                         
$ curl -H "Host:example.com" http://<your-ingress-ip-address>/
Hello World -- Rolling 01                                                  
$ curl -H "Host:example.com" http://<your-ingress-ip-address>/
Hello World -- This is rolling 02
```

For every 30 second, 20% more traffic will be shifted to the new instance from the old instance as we configured in Appfile.

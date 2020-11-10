# Setting Routes

Once your web services of the application deployed, you can visit it locally via `port-forward` or
from outside world via `route` feature. 

```bash
$ vela svc ls
NAME  	    APP  	WORKLOAD  	  TRAITS	STATUS 	    CREATED-TIME
frontend	testapp	webservice	      	    Deployed	2020-09-18 22:42:04 +0800 CST
```

## `port-forward`

It will directly open browser for you.

```bash
$ vela port-forward testapp
Forwarding from 127.0.0.1:8080 -> 80
Forwarding from [::1]:8080 -> 80

Forward successfully! Opening browser ...
Handling connection for 8080
Handling connection for 8080
```

## `route`

`route` is mainly used for public visiting your app.

### If you have didn't configure domain in environment

You can manually configure it by setting domain parameter.

```bash
$ vela route testapp --domain frontend.mycustom.domain
Adding route for app frontend

Rendering configs for service (frontend)...
⠋ Deploying ...
✅ Application Deployed Successfully!
Showing status of service(type: webservice) frontend deployed in Environment myenv
Service frontend Status:	 HEALTHY Ready: 1/1
	route: 	Visiting URL: http://frontend.mycustom.domain	IP: 123.57.10.233

Last Deployment:
	Created at: 2020-10-29 15:45:13 +0800 CST
	Updated at: 2020-10-29T16:12:45+08:00
```

Then you will be able to visit by:

```shell script
$ curl -H "Host:frontend.mycustom.domain" 123.57.10.233
```

### If you have domain set in environment

```bash
$ vela route testapp
Adding route for app frontend

Rendering configs for service (frontend)...
⠋ Deploying ...
✅ Application Deployed Successfully!
Showing status of service(type: webservice) frontend deployed in Environment default
Service frontend Status:	 HEALTHY Ready: 1/1
	route: 	Visiting URL: https://frontend.123.57.10.233.xip.io	IP: 123.57.10.233

Last Deployment:
	Created at: 2020-10-29 11:26:46 +0800 CST
	Updated at: 2020-10-29T11:28:01+08:00
```

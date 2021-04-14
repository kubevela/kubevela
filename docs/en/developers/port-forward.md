---
title:  Port Forwarding
---

Once your web services of the application deployed, you can access it locally via `port-forward`. 

```bash
$ vela ls
NAME  	        APP  	WORKLOAD  	  TRAITS	STATUS 	    CREATED-TIME
express-server	testapp	webservice	      	    Deployed	2020-09-18 22:42:04 +0800 CST
```

It will directly open browser for you.

```bash
$ vela port-forward testapp
Forwarding from 127.0.0.1:8080 -> 80
Forwarding from [::1]:8080 -> 80

Forward successfully! Opening browser ...
Handling connection for 8080
Handling connection for 8080
```
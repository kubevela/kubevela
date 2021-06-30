---
title:  端口转发
---

当你的 web 服务 Application 已经被部署就可以通过 `port-forward` 来本地访问。

```bash
$ vela ls
NAME  	        APP  	WORKLOAD  	  TRAITS	STATUS 	    CREATED-TIME
express-server	testapp	webservice	      	    Deployed	2020-09-18 22:42:04 +0800 CST
```

它将直接为你打开浏览器。

```bash
$ vela port-forward testapp
Forwarding from 127.0.0.1:8080 -> 80
Forwarding from [::1]:8080 -> 80

Forward successfully! Opening browser ...
Handling connection for 8080
Handling connection for 8080
```
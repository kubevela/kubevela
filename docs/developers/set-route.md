# Setting Routes

Once your web services of the application is deployed, you can visit it from outside world via `route` feature. 

## `route`

```console
$ vela svc ls
NAME  	    APP  	WORKLOAD  	  TRAITS	STATUS 	    CREATED-TIME
frontend	myapp	webservice	      	    Deployed	2020-09-18 22:42:04 +0800 CST
```

```console
$ vela route frontend --app myapp
  Adding route for app frontend
  Succeeded!

  Route information:

  HOSTS                	   ADDRESS         PORTS     AGE
  frontend.kubevela.demo   123.57.10.233   80, 443   73s
```

> TODO why don't use xip.io as demo?

Please configure `kubevela.demo` domain pointing to the public address (e.g. `123.57.10.233`) and your application can be then reached by `https://frontend.kubevela.demo`.

> You can achieve this by modifying `/etc/hosts` if your domain is fake.

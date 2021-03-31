---
title:  Setting Up Deployment Environment
---

A deployment environment is where you could configure the workspace, email for contact and domain for your applications globally.
A typical set of deployment environment is `test`, `staging`, `prod`, etc.

## Create environment

```bash
$ vela env init demo --email my@email.com
environment demo created, Namespace: default, Email: my@email.com
```

## Check the deployment environment metadata

```bash
$ vela env ls
NAME   	CURRENT	NAMESPACE	EMAIL                	DOMAIN
default	       	default  	
demo   	*      	default  	my@email.com
```

By default, the environment will use `default` namespace in K8s.

## Configure changes 

You could change the config by executing the environment again.

```bash
$ vela env init demo --namespace demo
environment demo created, Namespace: demo, Email: my@email.com
```

```bash
$ vela env ls
NAME   	CURRENT	NAMESPACE	EMAIL                	DOMAIN
default	       	default  	
demo   	*      	demo     	my@email.com
```

**Note that the created apps won't be affected, only newly created apps will use the updated info.**

## [Optional] Configure Domain if you have public IP

If your K8s cluster is provisioned by cloud provider and has public IP for ingress.
You could configure your domain in the environment, then you'll be able to visit
your app by this domain with an mTLS supported automatically.

For example, you could get the public IP from ingress service.  

```bash
$ kubectl get svc -A | grep LoadBalancer
NAME                         TYPE           CLUSTER-IP      EXTERNAL-IP     PORT(S)                      AGE
nginx-ingress-lb             LoadBalancer   172.21.2.174    123.57.10.233   80:32740/TCP,443:32086/TCP   41d
```

The fourth column is public IP. Configure 'A' record for your custom domain.

```
*.your.domain => 123.57.10.233
``` 

You could also use `123.57.10.233.xip.io` as your domain, if you don't have a custom one.
`xip.io` will automatically route to the prefix IP `123.57.10.233`.


```bash
$ vela env init demo --domain 123.57.10.233.xip.io
environment demo updated, Namespace: demo, Email: my@email.com
```

### Using domain in Appfile

Since you now have domain configured globally in deployment environment, you don't need to specify the domain in route configuration anymore.

```yaml
# in demo environment
servcies:
  express-server:
    ...

    route:
      rules:
        - path: /testapp
          rewriteTarget: /
```

```
$ curl http://123.57.10.233.xip.io/testapp
Hello World
```


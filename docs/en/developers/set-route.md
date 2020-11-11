# Setting Routes

The `route` section is used to configure the access to your app.

Add routing config under `express-server`:

```yaml
servcies:
  express-server:
    ...

    route:
      domain: example.com
      rules:
        - path: /testapp
          rewriteTarget: /
```

Apply again:

```bash
$ vela up
```

Check the status until we see route trait ready:
```bash
$ vela status testapp
About:

  Name:      	testapp
  Namespace: 	default
  Created at:	...
  Updated at:	...

Services:

  - Name: express-server
    Type: webservice
    HEALTHY Ready: 1/1
    Last Deployment:
      Created at: ...
      Updated at: ...
    Routes:
      - route: 	Visiting URL: http://example.com	IP: <ingress-IP-address>
```

**In [kind cluster setup](../../install.md#kind)**, you can visit the service via localhost:

> If not in kind cluster, replace localhost with ingress address

```
$ curl -H "Host:example.com" http://localhost/testapp
Hello World
```

---
title:  Setting Routes
---

The `route` section is used to configure the access to your app.

## Prerequisite
Make sure route trait controller is installed in your cluster

Install route trait controller with helm

1. Add helm chart repo for route trait
    ```shell script
    helm repo add oam.catalog  http://oam.dev/catalog/
    ```

2. Update the chart repo
    ```shell script
    helm repo update
    ```

3. Install route trait controller
    ```shell script
    helm install --create-namespace -n vela-system routetrait oam.catalog/routetrait


> Note: route is one of the extension capabilities [installed from cap center](../cap-center),
> please install it if you can't find it in `vela traits`.
   
## Setting route policy
Add routing config under `express-server`:

```yaml
services:
  express-server:
    ...

    route:
      domain: example.com
      rules:
        - path: /testapp
          rewriteTarget: /
```

> The full specification of `route` could show up by `$ vela show route` or be found on [its reference documentation](../references/traits/route)

Apply again:

```bash
$ vela up
```

Check the status until we see route is ready:
```bash
$ vela status testapp
About:

  Name:      	testapp
  Namespace: 	default
  Created at:	2020-11-04 16:34:43.762730145 -0800 PST
  Updated at:	2020-11-11 16:21:37.761158941 -0800 PST

Services:

  - Name: express-server
    Type: webservice
    HEALTHY Ready: 1/1
    Last Deployment:
      Created at: 2020-11-11 16:21:37 -0800 PST
      Updated at: 2020-11-11T16:21:37-08:00
    Routes:
      - route: 	Visiting URL: http://example.com	IP: <ingress-IP-address>
```

**In [kind cluster setup](../../install#kind)**, you can visit the service via localhost:

> If not in kind cluster, replace 'localhost' with ingress address

```
$ curl -H "Host:example.com" http://localhost/testapp
Hello World
```

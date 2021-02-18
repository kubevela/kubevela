# Building Developer Experience with KubeVela

For end users of KubeVela based platforms, we recommend platform builders serve them with simple and developer-centric interfaces instead of exposing full Kubernetes details. The common practices include building GUI console, adopting DSL, or creating a user friendly command line tool.

In KubeVela, we also provide a docker-compose style descriptor named `Appfile` with a single command named `vela up` as an example of building developer experience with KubeVela.

Let's walk through its experience first.

## Step 1: Install

Make sure you have finished and verified the installation following [this guide](./install.md).

## Step 2: Deploy Your First Application

```bash
$ vela up -f https://raw.githubusercontent.com/oam-dev/kubevela/master/docs/examples/vela.yaml
Parsing vela.yaml ...
Loading templates ...

Rendering configs for service (testsvc)...
Writing deploy config to (.vela/deploy.yaml)

Applying deploy configs ...
Checking if app has been deployed...
App has not been deployed, creating a new deployment...
✅ App has been deployed 🚀🚀🚀
    Port forward: vela port-forward first-vela-app
             SSH: vela exec first-vela-app
         Logging: vela logs first-vela-app
      App status: vela status first-vela-app
  Service status: vela status first-vela-app --svc testsvc
```

Check the status until we see `Routes` are ready:
```bash
$ vela status first-vela-app
About:

  Name:       first-vela-app
  Namespace:  default
  Created at: ...
  Updated at: ...

Services:

  - Name: testsvc
    Type: webservice
    HEALTHY Ready: 1/1
    Last Deployment:
      Created at: ...
      Updated at: ...
    Traits:
      - ✅ ingress: Visiting URL: testsvc.example.com, IP: <your IP address>
```

**In [kind cluster setup](./install.md#kind)**, you can visit the service via localhost. In other setups, replace localhost with ingress address accordingly.

```
$ curl -H "Host:testsvc.example.com" http://localhost/
<xmp>
Hello World


                                       ##         .
                                 ## ## ##        ==
                              ## ## ## ## ##    ===
                           /""""""""""""""""\___/ ===
                      ~~~ {~~ ~~~~ ~~~ ~~~~ ~~ ~ /  ===- ~~~
                           \______ o          _,/
                            \      \       _,'
                             `'--.._\..--''
</xmp>
```
**Voila!** You are all set to go.

## What's Next

- Learn details about [`Appfile`](./developers/learn-appfile.md) and know how it works.

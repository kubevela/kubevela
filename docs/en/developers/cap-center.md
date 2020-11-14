# Managing Capabilities

In KubeVela, developers can install more capabilities (i.e. new workload types and traits) from any GitHub repo that contains OAM definition files. We call these GitHub repos as _Capability Centers_. 

KubeVela is able to discover OAM definition files in this repo automatically and sync them to your own KubeVela platform.

## Add a capability center

Add and sync a capability center in KubeVela:

```bash
$ vela cap center config my-center https://github.com/oam-dev/catalog/tree/master/registry
successfully sync 1/1 from my-center remote center
Successfully configured capability center my-center and sync from remote

$ vela cap center sync my-center
successfully sync 1/1 from my-center remote center
sync finished
```

Now, this capability center `my-center` is ready to use.

## List capability centers

You are allowed to add more capability centers and list them.

```bash
$ vela cap center ls
NAME     	ADDRESS
my-center	https://github.com/oam-dev/catalog/tree/master/registry
```

## [Optional] Remove a capability center

Or, remove one.

```bash
$ vela cap center remove my-center
```

## List all available capabilities in capability center

Or, list all available capabilities in certain center.

```bash
$ vela cap ls my-center
NAME     	CENTER   	TYPE 	DEFINITION                  	STATUS     	APPLIES-TO
kubewatch	my-center	trait	kubewatches.labs.bitnami.com	uninstalled	[]
```

## Install a capability from capability center

Now let's try to install the new trait named `kubewatch` from `my-center` to your own KubeVela platform.

> [KubeWatch](https://github.com/bitnami-labs/kubewatch) is a Kubernetes plugin that watches events and publishes notifications to Slack channel etc. We can use it as a trait to watch important changes of your app and notify the platform administrators via Slack.

Install `kubewatch` trait from `my-center`.

```bash
$ vela cap install my-center/kubewatch
Installing trait capability kubewatch
"my-repo" has been added to your repositories
2020/11/06 16:19:30 [debug] creating 1 resource(s)
2020/11/06 16:19:30 [debug] CRD kubewatches.labs.bitnami.com is already present. Skipping.
2020/11/06 16:19:37 [debug] creating 3 resource(s)
Successfully installed chart (kubewatch) with release name (kubewatch)
Successfully installed capability kubewatch from my-center
```

## Use the newly installed capability

Let's check the `kubewatch` trait appears in your platform firstly:

```bash
$ vela traits
Synchronizing capabilities from cluster⌛ ...
Sync capabilities successfully ✅ (no changes)
TYPE      	CATEGORY	DESCRIPTION
kubewatch 	trait   	Add a watch for resource
...
```

Great! Now let's deploy an app via Appfile.


```bash
$ cat << EOF > vela.yaml
name: testapp
services:
  testsvc:
    type: webservice
    image: crccheck/hello-world
    port: 8000
    route:
      domain: testsvc.example.com
EOF
```

```bash
$ vela up
```

Then let's add `kubewatch` as a trait in this Appfile.

```bash
$ cat << EOF >> vela.yaml
    kubewatch:
      webhook: https://hooks.slack.com/<your-token>
EOF
```

> The `https://hooks.slack.com/<your-token>` is the Slack channel that your platform administrators are keeping an eye on.

Update the deployment:

```
$ vela up
```

Now, your platform administrators should receive notifications whenever important changes happen to your app. For example, a fresh new deployment.

![Image of Kubewatch](../../resources/kubewatch-notif.jpg)

## Uninstall a capability

> NOTE: make sure no apps are using the capability before uninstalling.

```bash
$ vela cap uninstall my-center/kubewatch
Successfully removed chart (kubewatch) with release name (kubewatch)
```

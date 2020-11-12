# Managing Capabilities

Developers can install more capabilities (workload types and traits) from any GitHub repo that contains OAM definition objects (i.e. capability center).

## Add Capability Center

Add and sync a remote center:

```bash
$ vela cap center config my-center https://github.com/oam-dev/catalog/tree/master/registry
successfully sync 1/1 from my-center remote center
Successfully configured capability center my-center and sync from remote

$ vela cap center sync my-center
successfully sync 1/1 from my-center remote center
sync finished
```

## List Capability Centers

```bash
$ vela cap center ls
NAME     	ADDRESS
my-center	https://github.com/oam-dev/catalog/tree/master/registry
```

## [Optional] Remove Cap Center

```bash
$ vela cap center remove my-center
```

## List Capabilities

```bash
$ vela cap ls my-center
NAME     	CENTER   	TYPE 	DEFINITION                  	STATUS     	APPLIES-TO
kubewatch	my-center	trait	kubewatches.labs.bitnami.com	uninstalled	[]
```

## Install Capability

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

Check traits installed:
```bash
$ vela traits
Synchronizing capabilities from cluster⌛ ...
Sync capabilities successfully ✅ (no changes)
TYPE      	CATEGORY	DESCRIPTION
kubewatch 	trait   	Add a watch for resource
...
```

## Uninstall Capability

> Note: make sure no apps are using the capability before uninstalling.

```bash
$ vela cap uninstall my-center/kubewatch
Successfully removed chart (kubewatch) with release name (kubewatch)
```

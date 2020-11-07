# Managing Capabilities

This tutorial talks about how to install capabilities (caps) from remote centers.

## Add Cap Center

Add and sync a remote center:

```console
$ vela cap center config my-center https://github.com/oam-dev/catalog/tree/master/registry
successfully sync 1/1 from my-center remote center
Successfully configured capability center my-center and sync from remote

$ vela cap center sync my-center
successfully sync 1/1 from my-center remote center
sync finished
```

## List Cap Centers

```console
$ vela cap center ls
NAME     	ADDRESS
my-center	https://github.com/oam-dev/catalog/tree/master/registry
```

## [Optional] Remove Cap Center

```console
$ vela cap center remove my-center
```

## List Caps

```console
$ vela cap ls my-center
NAME     	CENTER   	TYPE 	DEFINITION                  	STATUS     	APPLIES-TO
kubewatch	my-center	trait	kubewatches.labs.bitnami.com	uninstalled	[]
```

## Install Cap

```console
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
```console
$ vela traits
Synchronizing capabilities from cluster⌛ ...
Sync capabilities successfully ✅ (no changes)
TYPE      	CATEGORY	DESCRIPTION
kubewatch 	trait   	Add a watch for resource
...
```

## Uninstall Cap

> Note: make sure no apps are using the capability before uninstalling.

```console
$ vela cap uninstall my-center/kubewatch
Successfully removed chart (kubewatch) with release name (kubewatch)
```

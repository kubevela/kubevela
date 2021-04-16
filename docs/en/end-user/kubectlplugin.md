---
title: Install kubectl plugin
---

Install vela kubectl plugin can help you to ship applications more easily!

## Installation

You can install kubectl plugin `kubectl vela` by:

**macOS/Linux**
```shell script
curl -fsSl https://kubevela.io/script/install-kubectl-vela.sh | bash
```

You can also download the binary from [release pages ( >= v1.0.3)](https://github.com/oam-dev/kubevela/releases) manually.
Kubectl will discover it from your system path automatically.

## Usage

```shell
$ kubectl vela -h
A Highly Extensible Platform Engine based on Kubernetes and Open Application Model.

Usage:
  kubectl vela [flags]
  kubectl vela [command]

Available Commands:

Flags:
  -h, --help   help for vela

    dry-run  	Dry Run an application, and output the K8s resources as
             	result to stdout, only CUE template supported for now
    live-diff	Dry-run an application, and do diff on a specific app
             	revison. The provided capability definitions will be used
             	during Dry-run. If any capabilities used in the app are not
             	found in the provided ones, it will try to find from
             	cluster.
    show     	Show the reference doc for a workload type or trait
    version  	Prints out build version information


Use "kubectl vela [command] --help" for more information about a command.
```
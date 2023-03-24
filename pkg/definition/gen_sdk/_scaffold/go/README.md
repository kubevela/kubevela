# KubeVela Go SDK

This is a Go SDK for KubeVela generated via vela CLI

## Installation

Recommended way to install this SDK is run command below.

```shell
go get github.com/kubevela-contrib/kubevela-go-sdk
go mod edit -replace=sigs.k8s.io/apiserver-network-proxy/konnectivity-client=sigs.k8s.io/apiserver-network-proxy/konnectivity-client@v0.0.24
```

## Features:

- 🔧Application manipulating
  - [x] Add Components/Traits/Workflow Steps/Policies
  - [x] Set Workflow Mode
  - [x] Convert to/from K8s Application Object
  - [x] Convert to YAML/JSON
  - [x] Get Components/Traits/Workflow Steps/Policies from app
  - [x] Validate Application required parameters recursively
  - [ ] Referring to external Workflow object.
- 🔍Application client
  - [x] Create/Delete/Patch/Update Application
  - [x] List/Get Application

## Example

See [example](example) for some basic usage of this SDK.

## Future Work

There is some proper features for this SDK and possible to be added in the future. If you are interested in any of them, please feel free to contact us.

- Part of vela CLI functions
  - Application logs/exec/port-forward
  - Application resource in tree structure
  - VelaQL
  - ...
- Standalone workflow functions
  - CRUD of workflow

## Known issues

There are some known issues in this SDK, please be aware of them before using it.

1. `labels` and `annotations` trait is not working as expected.
2. `notification` workflow-step's parameter is not exactly the same as the one in KubeVela.

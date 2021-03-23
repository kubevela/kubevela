// +build generate

// See the below link for details on what is happening here.
// https://github.com/golang/go/wiki/Modules#how-can-i-track-tool-dependencies-for-a-module

// NOTE(@wonderflow) We don't remove existing CRDs here, because the crd folders contain not only auto generated.

// Generate deepcopy methodsets and CRD manifests
//go:generate go run -tags generate sigs.k8s.io/controller-tools/cmd/controller-gen object:headerFile=../hack/boilerplate.go.txt paths=./... crd:crdVersions=v1 output:artifacts:config=../config/crd/base

// Generate legacy_support for K8s 1.12~1.15 versions CRD manifests
//go:generate go run -tags generate sigs.k8s.io/controller-tools/cmd/controller-gen object:headerFile=../hack/boilerplate.go.txt paths=./... crd:trivialVersions=true output:artifacts:config=../legacy/charts/vela-core-legacy/crds
//go:generate go run ../legacy/convert/main.go ../legacy/charts/vela-core-legacy/crds

//go:generate go run ../hack/crd/update.go ../charts/vela-core/crds/standard.oam.dev_podspecworkloads.yaml

package apis

import (
	_ "sigs.k8s.io/controller-tools/cmd/controller-gen" //nolint:typecheck
)

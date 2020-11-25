// +build generate

// See the below link for details on what is happening here.
// https://github.com/golang/go/wiki/Modules#how-can-i-track-tool-dependencies-for-a-module

//go:generate go run ../hack/crd/update.go ../charts/vela-core/crds/

package apis

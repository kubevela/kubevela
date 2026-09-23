/*
Copyright 2026 The KubeVela Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package kuberead serves `vela/kube` to SourceDefinitions with the writes
// removed.
//
// The upstream package offers #Get, #List, #Apply and #Patch under one name. A
// source is a cached, shared read - one resolution serves every Application with
// the same key - so #Apply from a source writes to the cluster on every cache
// miss, under the controller's identity, from something an author bound
// expecting a read. Restricting the package set is not enough when a single
// package carries both.
//
// The name is still "kube", so `import "vela/kube"` in a source template
// resolves here and `kube.#Apply` is an undefined field, refused when the
// definition is applied rather than on its first resolve.
package kuberead

import (
	_ "embed"

	"github.com/kubevela/pkg/cue/cuex/providers/kube"
	cuexruntime "github.com/kubevela/pkg/cue/cuex/runtime"
	"github.com/kubevela/pkg/util/runtime"
)

// ProviderName is the name a source template imports as `vela/kube`.
const ProviderName = "kube"

//go:embed kube.cue
var template string

// Package is `vela/kube` with only the reading actions registered.
var Package = runtime.Must(cuexruntime.NewInternalPackage(ProviderName, template,
	map[string]cuexruntime.ProviderFn{
		"get":  cuexruntime.GenericProviderFn[kube.ResourceParams, kube.ResourceReturns](kube.Get),
		"list": cuexruntime.GenericProviderFn[kube.ListParams, kube.ListReturns](kube.List),
	}))

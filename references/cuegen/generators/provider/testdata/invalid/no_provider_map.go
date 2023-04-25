/*
Copyright 2023 The KubeVela Authors.

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

package invalid

import (
	"github.com/kubevela/pkg/cue/cuex/providers"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// ResourceVars .
type ResourceVars struct {
	Field1 string                     `json:"field1"`
	Field2 *unstructured.Unstructured `json:"field2"`
}

// ResourceParams is the params for resource
type ResourceParams providers.Params[ResourceVars]

// ResourceReturns is the returns for resource
type ResourceReturns providers.Returns[*unstructured.Unstructured]

// No provider map provided

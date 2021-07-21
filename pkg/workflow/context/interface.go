/*
Copyright 2021 The KubeVela Authors.

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

package context

import (
	runtimev1alpha1 "github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"

	"github.com/oam-dev/kubevela/pkg/cue/model/value"
)

// Context is workflow context interface
type Context interface {
	GetComponent(name string) (*ComponentManifest, error)
	PatchComponent(name string, patchValue *value.Value) error
	GetVar(paths ...string) (*value.Value, error)
	SetVar(v *value.Value, paths ...string) error
	Commit() error
	MakeParameter(parameter map[string]interface{}) (*value.Value, error)
	StoreRef() *runtimev1alpha1.TypedReference
}

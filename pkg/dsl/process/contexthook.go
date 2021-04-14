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

package process

import "github.com/oam-dev/kubevela/pkg/dsl/model"

// BaseHook defines function to be invoked before setting base to a
// process.Context
type BaseHook interface {
	Exec(Context, model.Instance) error
}

// BaseHookFn implements BaseHook interface
type BaseHookFn func(Context, model.Instance) error

// Exec will be invoked before settiing 'base' into ctx.Base
func (fn BaseHookFn) Exec(ctx Context, base model.Instance) error {
	return fn(ctx, base)
}

// AuxiliaryHook defines function to be invoked before appending auxiliaries to
// a process.Context
type AuxiliaryHook interface {
	Exec(Context, []Auxiliary) error
}

// AuxiliaryHookFn implements AuxiliaryHook interface
type AuxiliaryHookFn func(Context, []Auxiliary) error

// Exec will be invoked before appending 'auxs' into ctx.Auxiliaries
func (fn AuxiliaryHookFn) Exec(ctx Context, auxs []Auxiliary) error {
	return fn(ctx, auxs)
}

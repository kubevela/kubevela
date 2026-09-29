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

package celexpr

import (
	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"
	"github.com/google/cel-go/common/types/traits"

	"github.com/oam-dev/kubevela/pkg/definition/propexpr"
)

// placementFunctions declares cluster and namespace on a component read.
//
// Each only looks up what was delivered under the call in the receiver's
// propexpr.QualifiedKey entry. Resolving a placement needs the Application's
// topology and live cluster state, which an expression must not reach, so the
// caller resolves every placement the expression names before evaluating it.
func placementFunctions() []cel.EnvOption {
	lookup := func(fn string) func(recv, arg ref.Val) ref.Val {
		return func(recv, arg ref.Val) ref.Val {
			s, _ := arg.Value().(string)
			return delivered(recv, propexpr.PlacementCall(fn, s))
		}
	}
	return []cel.EnvOption{
		cel.Function(propexpr.PlaceCluster,
			cel.MemberOverload("vela_component_cluster", []*cel.Type{cel.DynType, cel.StringType}, cel.DynType,
				cel.BinaryBinding(lookup(propexpr.PlaceCluster)))),
		cel.Function(propexpr.PlaceNamespace,
			cel.MemberOverload("vela_component_namespace", []*cel.Type{cel.DynType, cel.StringType}, cel.DynType,
				cel.BinaryBinding(lookup(propexpr.PlaceNamespace)))),
	}
}

func delivered(recv ref.Val, call string) ref.Val {
	fn, arg := propexpr.SplitPlacementCall(call)
	notDelivered := types.NewErr("component read .%s(%q) was not delivered to this render", fn, arg)
	m, ok := recv.(traits.Mapper)
	if !ok {
		return notDelivered
	}
	views, found := m.Find(types.String(propexpr.QualifiedKey))
	if !found {
		return notDelivered
	}
	vm, ok := views.(traits.Mapper)
	if !ok {
		return notDelivered
	}
	v, found := vm.Find(types.String(call))
	if !found {
		return notDelivered
	}
	return v
}

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
package application

import (
	"errors"

	celengine "github.com/kubevela/pkg/cel"
	"k8s.io/apimachinery/pkg/util/validation/field"

	"github.com/oam-dev/kubevela/pkg/definition/celexpr"
)

// leafPlan is one properties blob as admission addresses it, leaves with field
// paths, beside the plan of its expressions, so every check reads the
// expressions once rather than parsing each leaf again.
type leafPlan struct {
	leaves []inputLeaf
	plan   *celengine.Plan
	// faults are the expressions that do not parse or compile; the plan is
	// nil when there are any.
	faults  celengine.CheckErrors
	byProp  map[string][]celengine.Expression
	leafFor map[string]inputLeaf
	// malformed is set when the blob is not JSON, and nothing else is.
	malformed error
}

func planLeaves(raw []byte, base *field.Path) leafPlan {
	plan, err := celexpr.Vela.PlanJSON(raw)
	var faults celengine.CheckErrors
	if err != nil && !errors.As(err, &faults) {
		return leafPlan{malformed: err}
	}
	lp := leafPlan{leaves: flattenLeafPaths(raw, base), leafFor: map[string]inputLeaf{}, faults: faults}
	for _, lf := range lp.leaves {
		lp.leafFor[lf.property] = lf
	}
	// A blob with no expression has nothing to check and no plan: the passes
	// skip it without loading the definition it feeds.
	if plan == nil || len(plan.Expressions()) == 0 {
		return lp
	}
	lp.plan = plan
	lp.byProp = map[string][]celengine.Expression{}
	for _, x := range plan.Expressions() {
		lp.byProp[x.Property] = append(lp.byProp[x.Property], x)
	}
	return lp
}

// fieldPath is where a fault at property sits, or base when it is not a leaf.
func (lp leafPlan) fieldPath(property string, base *field.Path) *field.Path {
	if lf, ok := lp.leafFor[property]; ok {
		return lf.fieldPath
	}
	return base
}

// leaf is the leaf at property.
func (lp leafPlan) leaf(property string) (inputLeaf, bool) {
	lf, ok := lp.leafFor[property]
	return lf, ok
}

// leafText is the leaf's value as written, for an error beside it.
func (lp leafPlan) leafText(property string) interface{} {
	if lf, ok := lp.leafFor[property]; ok {
		return lf.literal
	}
	return property
}

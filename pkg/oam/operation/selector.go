/*
 Copyright 2026. The KubeVela Authors.

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

// Package operation holds logic shared between the Operation controller
// (pkg/controller/core.oam.dev/v2alpha1/operation) and the `vela operation`
// CLI (references/cli/operation.go), so the two don't each carry their own
// copy of the same matching rule.
package operation

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v2alpha1"
)

// MatchesApplicationSelector reports a non-nil error naming why app doesn't
// match sel, or nil if it matches. RequiredComponentTypes is checked
// against app.Spec.Components directly rather than a parsed appfile, so the
// check doesn't depend on any prior resolution having happened.
func MatchesApplicationSelector(app *v1beta1.Application, sel *v2alpha1.OperationApplicationSelector) error {
	labelSelector := &metav1.LabelSelector{
		MatchLabels:      sel.MatchLabels,
		MatchExpressions: sel.MatchExpressions,
	}
	selector, err := metav1.LabelSelectorAsSelector(labelSelector)
	if err != nil {
		return fmt.Errorf("invalid operation template selector: %w", err)
	}
	if !selector.Matches(labels.Set(app.Labels)) {
		return fmt.Errorf("application %q does not match operation template's selector", app.Name)
	}
	for _, want := range sel.RequiredComponentTypes {
		found := false
		for _, c := range app.Spec.Components {
			if c.Type == want {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("application %q is missing required component type %q", app.Name, want)
		}
	}
	return nil
}

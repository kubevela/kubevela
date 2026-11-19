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
	"fmt"

	utilfeature "k8s.io/apiserver/pkg/util/feature"

	"github.com/oam-dev/kubevela/pkg/features"
	"github.com/oam-dev/kubevela/pkg/oam"
)

// conditionsDisabledMessage says which of the two switches is off. The workflow
// refuses the condition; which switch to point the user at is KubeVela's to say.
func conditionsDisabledMessage() string {
	if !utilfeature.DefaultMutableFeatureGate.Enabled(features.EnableCelExpressions) {
		return "a $( ) condition needs expressions, which are not enabled on this cluster; " +
			"an operator enables them with the EnableCelExpressions feature gate"
	}
	return fmt.Sprintf("a $( ) condition needs expressions, and this Application has not opted in to them; "+
		"set the %s annotation to \"true\"", oam.AnnotationCelExpressions)
}

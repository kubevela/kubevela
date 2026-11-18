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

package operation

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v2alpha1"
)

func TestMatchesApplicationSelector(t *testing.T) {
	app := &v1beta1.Application{
		ObjectMeta: metav1.ObjectMeta{Name: "myapp", Labels: map[string]string{"env": "prod"}},
		Spec: v1beta1.ApplicationSpec{
			Components: []common.ApplicationComponent{{Name: "web", Type: "webservice"}},
		},
	}

	assert.NoError(t, MatchesApplicationSelector(app, &v2alpha1.OperationApplicationSelector{}))
	assert.NoError(t, MatchesApplicationSelector(app, &v2alpha1.OperationApplicationSelector{MatchLabels: map[string]string{"env": "prod"}}))
	assert.Error(t, MatchesApplicationSelector(app, &v2alpha1.OperationApplicationSelector{MatchLabels: map[string]string{"env": "staging"}}))
	assert.NoError(t, MatchesApplicationSelector(app, &v2alpha1.OperationApplicationSelector{RequiredComponentTypes: []string{"webservice"}}))
	assert.Error(t, MatchesApplicationSelector(app, &v2alpha1.OperationApplicationSelector{RequiredComponentTypes: []string{"worker"}}))
	assert.Error(t, MatchesApplicationSelector(app, &v2alpha1.OperationApplicationSelector{MatchExpressions: []metav1.LabelSelectorRequirement{
		{Key: "bogus", Operator: metav1.LabelSelectorOpExists},
	}}))
	assert.NoError(t, MatchesApplicationSelector(app, &v2alpha1.OperationApplicationSelector{MatchExpressions: []metav1.LabelSelectorRequirement{
		{Key: "env", Operator: metav1.LabelSelectorOpIn, Values: []string{"prod"}},
	}}))
	assert.Error(t, MatchesApplicationSelector(app, &v2alpha1.OperationApplicationSelector{MatchExpressions: []metav1.LabelSelectorRequirement{
		{Key: "env", Operator: metav1.LabelSelectorOpIn, Values: []string{"staging"}},
	}}))
}

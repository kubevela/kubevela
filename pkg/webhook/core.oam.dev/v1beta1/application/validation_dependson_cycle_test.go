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
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	featuregatetesting "k8s.io/component-base/featuregate/testing"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/features"
)

// A cycle of dependsOn has nothing to do with expressions, so it is refused
// whichever way an Application stands on them.
//
// Both cases below are configurations a cluster really runs: expressions off is
// the shipped default, and opt-in required with no annotation is what every
// Application looks like on a cluster that has turned them on.
func TestValidateSourcesRefusesADependsOnCycleWithoutExpressions(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1beta1.AddToScheme(scheme)

	spec := v1beta1.ApplicationSpec{Components: []common.ApplicationComponent{
		{Name: "a", Type: "webservice", Properties: rawJSON(`{"image":"a"}`), DependsOn: []string{"b"}},
		{Name: "b", Type: "webservice", Properties: rawJSON(`{"image":"b"}`), DependsOn: []string{"a"}},
	}}

	for _, tc := range []struct {
		name  string
		setup func(*testing.T)
	}{
		{"expressions off", func(t *testing.T) {
			featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.EnableCelExpressions, false)
		}},
		{"opt-in required and not taken", func(t *testing.T) {
			featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.RequireCelExpressionOptIn, true)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.setup(t)
			app := &v1beta1.Application{
				ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "default"},
				Spec:       spec,
			}
			h := &ValidatingHandler{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}
			var joined string
			for _, e := range h.ValidateSources(context.Background(), app) {
				joined += e.Error() + "\n"
			}
			require.Contains(t, joined, "wait on each other in a cycle",
				"a dependsOn cycle must be refused, got:\n%s", joined)
		})
	}
}

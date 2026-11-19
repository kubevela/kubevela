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
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	wfv1alpha1 "github.com/kubevela/pkg/apis/oam/v1alpha1"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

func TestValidateComponentOutputReads(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1beta1.AddToScheme(scheme)

	db := common.ApplicationComponent{Name: "db", Type: "webservice", Properties: rawJSON(`{"image":"postgres"}`)}
	app := func(spec v1beta1.ApplicationSpec) *v1beta1.Application {
		return &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "default"}, Spec: spec}
	}

	for _, tc := range []struct {
		name    string
		app     *v1beta1.Application
		wantMsg string
	}{
		{
			name: "a component reads another's output",
			app: app(v1beta1.ApplicationSpec{Components: []common.ApplicationComponent{db,
				{Name: "api", Type: "webservice", Properties: rawJSON(`{"image":"api","cmd":["$(component.db.output.status.endpoint)"]}`)}}}),
		},
		{
			name: "a trait reads another component's output",
			app: app(v1beta1.ApplicationSpec{Components: []common.ApplicationComponent{db,
				{Name: "api", Type: "webservice", Properties: rawJSON(`{"image":"api"}`),
					Traits: []common.ApplicationTrait{{Type: "labels", Properties: rawJSON(`{"db":"$(component.db.output.metadata.name)"}`)}}}}}),
		},
		{
			name: "an unknown component",
			app: app(v1beta1.ApplicationSpec{Components: []common.ApplicationComponent{db,
				{Name: "api", Type: "webservice", Properties: rawJSON(`{"image":"$(component.cache.output.status.x)"}`)}}}),
			wantMsg: `no component "cache"`,
		},
		{
			name: "a workflow step cannot",
			app: app(v1beta1.ApplicationSpec{Components: []common.ApplicationComponent{db},
				Workflow: &v1beta1.Workflow{Steps: []wfv1alpha1.WorkflowStep{{WorkflowStepBase: wfv1alpha1.WorkflowStepBase{Name: "s", Type: "notification",
					Properties: rawJSON(`{"msg":"$(component.db.output.status.endpoint)"}`)}}}}}),
			wantMsg: `"component" cannot be read here`,
		},
		{
			name: "a source cannot",
			app: app(v1beta1.ApplicationSpec{Components: []common.ApplicationComponent{db},
				Sources: []v1beta1.ApplicationSource{{Name: "s", Type: "t", Properties: rawJSON(`{"x":"$(component.db.output.status.endpoint)"}`)}}}),
			wantMsg: `"component" cannot be read here`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := &ValidatingHandler{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}
			errs := h.ValidateSources(context.Background(), tc.app)
			var joined string
			for _, e := range errs {
				joined += e.Error() + "\n"
			}
			if tc.wantMsg == "" {
				if len(errs) != 0 {
					t.Fatalf("expected no errors, got:\n%s", joined)
				}
				return
			}
			if !strings.Contains(joined, tc.wantMsg) {
				t.Fatalf("expected an error containing %q, got:\n%s", tc.wantMsg, joined)
			}
		})
	}
}

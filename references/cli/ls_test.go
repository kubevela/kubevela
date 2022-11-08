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

package cli

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	common2 "github.com/oam-dev/kubevela/pkg/utils/common"
)

var componentOrderSpec = v1beta1.ApplicationSpec{
	Components: []common.ApplicationComponent{{
		Name:       "test-component1",
		Type:       "worker",
		Properties: &runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox"}`)},
	}, {
		Name:       "test-component2",
		Type:       "worker",
		Properties: &runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox"}`)},
	}, {
		Name:       "test-component3",
		Type:       "worker",
		Properties: &runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox"}`)},
	},
	},
}

var componentOrderStatus = common.AppStatus{
	Services: []common.ApplicationComponentStatus{{
		Name:    "test-component2",
		Message: "test-component2 applied",
		Healthy: true,
	}, {
		Name:    "test-component1",
		Message: "test-component1 applied",
		Healthy: false,
	}, {
		Name:    "test-component3",
		Message: "",
		Healthy: true,
	}},

	Phase: common.ApplicationRunning,
}

var componentSpec = v1beta1.ApplicationSpec{
	Components: []common.ApplicationComponent{
		{
			Name:       "test1-component",
			Type:       "worker",
			Properties: &runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox"}`)},
		},
	},
}

var componentStatus = common.AppStatus{
	Services: []common.ApplicationComponentStatus{
		{
			Name:    "test1-component",
			Message: "test1-component applied",
			Healthy: true,
		},
	},
	Phase: common.ApplicationRunning,
}

func TestBuildApplicationListTable(t *testing.T) {
	ctx := context.TODO()
	testCases := map[string]struct {
		apps              []*v1beta1.Application
		expectedErr       error
		namespace         string
		labelSelector     string
		fieldSelector     string
		expectAppListSize int
	}{
		"specified component order different from applied": {
			apps: []*v1beta1.Application{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "dependency",
						Namespace: "test",
					},
					Spec:   componentOrderSpec,
					Status: componentOrderStatus,
				},
			},
			expectedErr:       nil,
			namespace:         "test",
			expectAppListSize: 1,
		},
		"specified label selector": {
			apps: []*v1beta1.Application{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "app1",
						Namespace: "test1",
					},
					Spec:   componentSpec,
					Status: componentStatus,
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "app2",
						Namespace: "test1",
						Labels: map[string]string{
							"app.kubernetes.io/name":    "busybox",
							"app.kubernetes.io/version": "v1",
						},
					},
					Spec:   componentSpec,
					Status: componentStatus,
				},
			},
			expectedErr:       nil,
			namespace:         "test1",
			labelSelector:     "app.kubernetes.io/name=busybox,app.kubernetes.io/version=v1",
			expectAppListSize: 1,
		},
		"specified field selector": {
			apps: []*v1beta1.Application{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "app1",
						Namespace: "test2",
					},
					Spec:   componentSpec,
					Status: componentStatus,
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "app2",
						Namespace: "test2",
					},
					Spec:   componentSpec,
					Status: componentStatus,
				},
			},
			expectedErr:       nil,
			namespace:         "test2",
			fieldSelector:     "metadata.name=app2,metadata.namespace=test2",
			expectAppListSize: 1,
		},
	}

	client := fake.NewClientBuilder().WithScheme(common2.Scheme).Build()

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			r := require.New(t)

			if tc.apps != nil && len(tc.apps) > 0 {
				for _, app := range tc.apps {
					err := client.Create(ctx, app)
					r.NoError(err)
				}
			}
			service := map[string]common.ApplicationComponentStatus{}
			for _, app := range tc.apps {
				for _, s := range app.Status.Services {
					service[s.Name] = s
				}
			}

			LabelSelector = tc.labelSelector
			FieldSelector = tc.fieldSelector
			tb, err := buildApplicationListTable(ctx, client, tc.namespace)
			r.Equal(tc.expectedErr, err)
			for _, app := range tc.apps {
				for i, component := range app.Spec.Components {
					row := tb.Rows[i+1]
					compName := fmt.Sprintf("%s", row.Cells[1].Data)
					r.Equal(component.Name, compName)
					r.Equal(component.Type, fmt.Sprintf("%s", row.Cells[2].Data))
					r.Equal(string(app.Status.Phase), fmt.Sprintf("%s", row.Cells[4].Data))
					r.Equal(getHealthString(service[compName].Healthy), fmt.Sprintf("%s", row.Cells[5].Data))
					r.Equal(service[compName].Message, fmt.Sprintf("%s", row.Cells[6].Data))
				}
			}
			// filter header, "├─" and "└─" to get actual appList size
			var actualAppListSize int
			for _, row := range tb.Rows {
				if row.Cells[0].Data != "APP" && row.Cells[0].Data != "├─" && row.Cells[0].Data != "└─" {
					actualAppListSize = actualAppListSize + 1
				}
			}
			r.Equal(tc.expectAppListSize, actualAppListSize)
		})
	}
}

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

func TestBuildApplicationListTable(t *testing.T) {
	ctx := context.TODO()
	testCases := map[string]struct {
		app         *v1beta1.Application
		expectedErr error
	}{
		"specified component order different from applied": {
			app: &v1beta1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "dependency",
					Namespace: "test",
				},
				Spec:   componentOrderSpec,
				Status: componentOrderStatus,
			},
			expectedErr: nil,
		},
	}

	client := fake.NewClientBuilder().WithScheme(common2.Scheme).Build()

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			r := require.New(t)

			if tc.app != nil {
				err := client.Create(ctx, tc.app)
				r.NoError(err)
			}
			service := map[string]common.ApplicationComponentStatus{}
			for _, s := range tc.app.Status.Services {
				service[s.Name] = s
			}

			tb, err := buildApplicationListTable(ctx, client, "test")
			r.Equal(tc.expectedErr, err)

			for i, component := range tc.app.Spec.Components {
				row := tb.Rows[i+1]
				compName := fmt.Sprintf("%s", row.Cells[1].Data)
				r.Equal(component.Name, compName)
				r.Equal(component.Type, fmt.Sprintf("%s", row.Cells[2].Data))
				r.Equal(string(tc.app.Status.Phase), fmt.Sprintf("%s", row.Cells[4].Data))
				r.Equal(getHealthString(service[compName].Healthy), fmt.Sprintf("%s", row.Cells[5].Data))
				r.Equal(service[compName].Message, fmt.Sprintf("%s", row.Cells[6].Data))
			}
		})
	}
}

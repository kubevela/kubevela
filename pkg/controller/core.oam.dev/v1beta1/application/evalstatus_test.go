/*
Copyright 2024 The KubeVela Authors.

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
	"fmt"
	"testing"

	"cuelang.org/go/cue"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	monitorContext "github.com/kubevela/pkg/monitor/context"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

func Test_applyComponentHealthToServices(t *testing.T) {
	tests := []struct {
		name            string
		components      []common.ApplicationComponent
		services        []common.ApplicationComponentStatus
		healthCheckFunc func(string) *common.ApplicationComponentStatus
		verifyFunc      func(*testing.T, []common.ApplicationComponentStatus)
	}{
		{
			name: "each service gets its matching component's health status",
			components: []common.ApplicationComponent{
				{Name: "frontend", Type: "webservice", Properties: &runtime.RawExtension{Raw: []byte(`{}`)}},
				{Name: "backend", Type: "webservice", Properties: &runtime.RawExtension{Raw: []byte(`{}`)}},
				{Name: "database", Type: "webservice", Properties: &runtime.RawExtension{Raw: []byte(`{}`)}},
			},
			services: []common.ApplicationComponentStatus{
				{Name: "frontend", Namespace: "default", Cluster: "local"},
				{Name: "backend", Namespace: "default", Cluster: "local"},
				{Name: "database", Namespace: "default", Cluster: "local"},
			},
			healthCheckFunc: func(name string) *common.ApplicationComponentStatus {
				switch name {
				case "frontend":
					return &common.ApplicationComponentStatus{
						Healthy: true,
						Message: "frontend is healthy",
					}
				case "backend":
					return &common.ApplicationComponentStatus{
						Healthy: false,
						Message: "backend is unhealthy",
					}
				case "database":
					return &common.ApplicationComponentStatus{
						Healthy: true,
						Message: "database is healthy",
					}
				default:
					return nil
				}
			},
			verifyFunc: func(t *testing.T, services []common.ApplicationComponentStatus) {
				assert.True(t, services[0].Healthy, "frontend service should be healthy")
				assert.Equal(t, "frontend is healthy", services[0].Message)
				assert.False(t, services[1].Healthy, "backend service should be unhealthy")
				assert.Equal(t, "backend is unhealthy", services[1].Message)
				assert.True(t, services[2].Healthy, "database service should be healthy")
				assert.Equal(t, "database is healthy", services[2].Message)
			},
		},
		{
			name: "unmatched services remain unchanged",
			components: []common.ApplicationComponent{
				{Name: "app", Type: "webservice", Properties: &runtime.RawExtension{Raw: []byte(`{}`)}},
			},
			services: []common.ApplicationComponentStatus{
				{Name: "app", Namespace: "default", Cluster: "local"},
				{Name: "orphan", Namespace: "default", Cluster: "local", Healthy: true, Message: "pre-existing"},
			},
			healthCheckFunc: func(name string) *common.ApplicationComponentStatus {
				if name == "app" {
					return &common.ApplicationComponentStatus{
						Healthy: false,
						Message: "app checked",
					}
				}
				return nil
			},
			verifyFunc: func(t *testing.T, services []common.ApplicationComponentStatus) {
				assert.False(t, services[0].Healthy, "app service should be unhealthy")
				assert.Equal(t, "app checked", services[0].Message)
				assert.True(t, services[1].Healthy, "orphan service should remain healthy")
				assert.Equal(t, "pre-existing", services[1].Message)
			},
		},
		{
			name: "performance test - 100 components and services",
			components: func() []common.ApplicationComponent {
				comps := make([]common.ApplicationComponent, 100)
				for i := 0; i < 100; i++ {
					comps[i] = common.ApplicationComponent{
						Name:       fmt.Sprintf("comp-%d", i),
						Type:       "webservice",
						Properties: &runtime.RawExtension{Raw: []byte(`{}`)},
					}
				}
				return comps
			}(),
			services: func() []common.ApplicationComponentStatus {
				svcs := make([]common.ApplicationComponentStatus, 100)
				for i := 0; i < 100; i++ {
					svcs[i] = common.ApplicationComponentStatus{
						Name:      fmt.Sprintf("comp-%d", i),
						Namespace: "default",
						Cluster:   "local",
					}
				}
				return svcs
			}(),
			healthCheckFunc: func(name string) *common.ApplicationComponentStatus {
				return &common.ApplicationComponentStatus{
					Healthy: true,
					Message: name + " is healthy",
				}
			},
			verifyFunc: func(t *testing.T, services []common.ApplicationComponentStatus) {
				for i, svc := range services {
					expectedMsg := fmt.Sprintf("comp-%d is healthy", i)
					assert.True(t, svc.Healthy, "Service %d should be healthy", i)
					assert.Equal(t, expectedMsg, svc.Message, "Service %d message", i)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := monitorContext.NewTraceContext(context.Background(), "test")

			app := &v1beta1.Application{
				Spec: v1beta1.ApplicationSpec{
					Components: tt.components,
				},
			}

			handler := &AppHandler{
				app:      app,
				services: make([]common.ApplicationComponentStatus, len(tt.services)),
			}
			copy(handler.services, tt.services)

			componentMap := make(map[string]common.ApplicationComponent, len(tt.components))
			for _, component := range tt.components {
				componentMap[component.Name] = component
			}

			mockHealthCheck := func(ctx context.Context, comp common.ApplicationComponent, patcher *cue.Value, clusterName string, overrideNamespace string) (bool, *common.ApplicationComponentStatus, *unstructured.Unstructured, []*unstructured.Unstructured, error) {
				status := tt.healthCheckFunc(comp.Name)
				return false, status, nil, nil, nil
			}

			applyComponentHealthToServices(ctx, handler, componentMap, mockHealthCheck)

			if tt.verifyFunc != nil {
				tt.verifyFunc(t, handler.services)
			}
		})
	}
}

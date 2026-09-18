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
	"errors"
	"fmt"
	"strings"
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
		healthCheckErr  error
		// paramsSuppliedAtRuntime mirrors an Application whose components take a
		// parameter from a workflow step input, which the health check cannot render.
		paramsSuppliedAtRuntime bool
		verifyFunc              func(*testing.T, []common.ApplicationComponentStatus)
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
		{
			// The same failure on an Application whose parameters come from the
			// workflow is expected: the health check renders the component without
			// those values, so it cannot succeed and says nothing about the component.
			name: "a health check that fails is ignored when parameters come from the workflow",
			components: []common.ApplicationComponent{
				{Name: "myweb", Type: "webservice", Properties: &runtime.RawExtension{Raw: []byte(`{}`)}},
			},
			services: []common.ApplicationComponentStatus{
				{Name: "myweb", Namespace: "default", Cluster: "local", Healthy: true, Message: "applied by the workflow"},
			},
			healthCheckErr:          errors.New("GenerateComponentManifest: parameter.image: incomplete value string"),
			paramsSuppliedAtRuntime: true,
			verifyFunc: func(t *testing.T, services []common.ApplicationComponentStatus) {
				assert.True(t, services[0].Healthy, "the status the workflow recorded should stand")
				assert.Equal(t, "applied by the workflow", services[0].Message)
			},
		},
		{
			// A health check that could not run is not evidence of health. This is
			// how a type: addon or type: module component whose registry credentials
			// expired kept reporting healthy while every reconcile logged the
			// failure to fetch it.
			name: "a health check that fails marks the service unhealthy with the reason",
			components: []common.ApplicationComponent{
				{Name: "my-addon", Type: "addon", Properties: &runtime.RawExtension{Raw: []byte(`{}`)}},
			},
			services: []common.ApplicationComponentStatus{
				{Name: "my-addon", Namespace: "default", Cluster: "local", Healthy: true, Message: "addon application is running"},
			},
			healthCheckErr: errors.New("GenerateComponentManifest: evaluate: addon \"nest-module-poc\" not found in registries [my-addons]\n(value: #do: \"render\")"),
			verifyFunc: func(t *testing.T, services []common.ApplicationComponentStatus) {
				assert.False(t, services[0].Healthy, "a component whose health could not be checked is not healthy")
				assert.Equal(t, `GenerateComponentManifest: evaluate: addon "nest-module-poc" not found in registries [my-addons]`, services[0].Message)
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
				if tt.healthCheckErr != nil {
					return false, nil, nil, nil, tt.healthCheckErr
				}
				status := tt.healthCheckFunc(comp.Name)
				return false, status, nil, nil, nil
			}

			applyComponentHealthToServices(ctx, handler, componentMap, mockHealthCheck, tt.paramsSuppliedAtRuntime)

			if tt.verifyFunc != nil {
				tt.verifyFunc(t, handler.services)
			}
		})
	}
}

func TestFilterRemovedComponentsFromStatus(t *testing.T) {
	tests := []struct {
		name             string
		components       []common.ApplicationComponent
		statusServices   []common.ApplicationComponentStatus
		expectedServices []string
		servicesRemoved  bool
	}{
		{
			name: "removed component is filtered from services",
			components: []common.ApplicationComponent{
				{Name: "backend", Type: "webservice"},
			},
			statusServices: []common.ApplicationComponentStatus{
				{Name: "frontend", Namespace: "default"},
				{Name: "backend", Namespace: "default"},
			},
			expectedServices: []string{"backend"},
			servicesRemoved:  true,
		},
		{
			name:       "all components removed results in empty services",
			components: []common.ApplicationComponent{},
			statusServices: []common.ApplicationComponentStatus{
				{Name: "frontend", Namespace: "default"},
				{Name: "backend", Namespace: "default"},
			},
			expectedServices: []string{},
			servicesRemoved:  true,
		},
		{
			name: "no components removed keeps all services",
			components: []common.ApplicationComponent{
				{Name: "frontend", Type: "webservice"},
				{Name: "backend", Type: "webservice"},
			},
			statusServices: []common.ApplicationComponentStatus{
				{Name: "frontend", Namespace: "default"},
				{Name: "backend", Namespace: "default"},
			},
			expectedServices: []string{"frontend", "backend"},
			servicesRemoved:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filteredServices, servicesRemoved := filterRemovedComponentsFromStatus(
				tt.components,
				tt.statusServices,
			)

			assert.Equal(t, tt.servicesRemoved, servicesRemoved,
				"servicesRemoved flag should match expected value")

			assert.Equal(t, len(tt.expectedServices), len(filteredServices),
				"filtered services count should match expected")
			for i, expectedName := range tt.expectedServices {
				assert.Equal(t, expectedName, filteredServices[i].Name,
					fmt.Sprintf("service at index %d should be %s", i, expectedName))
			}
		})
	}
}

func TestHealthCheckErrorMessage(t *testing.T) {
	testCases := map[string]struct {
		err  error
		want string
	}{
		"only the first line is kept, because the rest is the failing CUE value": {
			err:  errors.New("failed to compile workload my-addon: addon not exist\n(value: #do: \"render\"\n$params: {})"),
			want: `failed to compile workload my-addon: addon not exist`,
		},
		"a single-line error is passed through": {
			err:  errors.New("registry my-addons: 401 Unauthorized"),
			want: "registry my-addons: 401 Unauthorized",
		},
		"a long error is capped so it is not patched onto the Application in full": {
			err:  errors.New(strings.Repeat("x", maxHealthCheckErrorMessage+50)),
			want: strings.Repeat("x", maxHealthCheckErrorMessage) + "...",
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, healthCheckErrorMessage(tc.err))
		})
	}
}

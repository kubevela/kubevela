/*
Copyright 2025 The KubeVela Authors.

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

	"github.com/kubevela/pkg/controller/sharding"
	"github.com/stretchr/testify/assert"
	admissionv1 "k8s.io/api/admission/v1"
	authenticationv1 "k8s.io/api/authentication/v1"
	authv1 "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	featuregatetesting "k8s.io/component-base/featuregate/testing"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	wfTypesv1alpha1 "github.com/kubevela/pkg/apis/oam/v1alpha1"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/features"
	"github.com/oam-dev/kubevela/pkg/oam"
)

func TestValidateCreate(t *testing.T) {
	// Disable the definition validation feature for this test
	featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultMutableFeatureGate, features.ValidateDefinitionPermissions, false)

	// Enable sharding to skip component validation (since we don't have component definitions in tests)
	oldSharding := sharding.EnableSharding
	sharding.EnableSharding = true
	defer func() { sharding.EnableSharding = oldSharding }()

	scheme := runtime.NewScheme()
	_ = v1beta1.AddToScheme(scheme)
	_ = authv1.AddToScheme(scheme)

	testCases := []struct {
		name               string
		app                *v1beta1.Application
		expectedErrorCount int
		expectedErrorField string
	}{
		{
			name: "valid application",
			app: &v1beta1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-app",
					Namespace: "default",
				},
				Spec: v1beta1.ApplicationSpec{
					Components: []common.ApplicationComponent{
						{
							Name: "comp1",
							Type: "webservice",
						},
					},
				},
			},
			expectedErrorCount: 0,
		},
		{
			name: "application with both autoUpdate and publishVersion",
			app: &v1beta1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-app",
					Namespace: "default",
					Annotations: map[string]string{
						oam.AnnotationAutoUpdate:     "true",
						oam.AnnotationPublishVersion: "v1.0.0",
					},
				},
				Spec: v1beta1.ApplicationSpec{
					Components: []common.ApplicationComponent{
						{
							Name: "comp1",
							Type: "webservice",
						},
					},
				},
			},
			expectedErrorCount: 1,
			expectedErrorField: "metadata.annotations",
		},
		{
			name: "application with duplicate workflow step names",
			app: &v1beta1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-app",
					Namespace: "default",
				},
				Spec: v1beta1.ApplicationSpec{
					Components: []common.ApplicationComponent{
						{
							Name: "comp1",
							Type: "webservice",
						},
					},
					Workflow: &v1beta1.Workflow{
						Steps: []wfTypesv1alpha1.WorkflowStep{
							{
								WorkflowStepBase: wfTypesv1alpha1.WorkflowStepBase{
									Name: "step1",
									Type: "deploy",
								},
							},
							{
								WorkflowStepBase: wfTypesv1alpha1.WorkflowStepBase{
									Name: "step1", // Duplicate name
									Type: "deploy",
								},
							},
						},
					},
				},
			},
			expectedErrorCount: 1,
			expectedErrorField: "spec.workflow.steps",
		},
		{
			name: "application with invalid timeout",
			app: &v1beta1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-app",
					Namespace: "default",
				},
				Spec: v1beta1.ApplicationSpec{
					Components: []common.ApplicationComponent{
						{
							Name: "comp1",
							Type: "webservice",
						},
					},
					Workflow: &v1beta1.Workflow{
						Steps: []wfTypesv1alpha1.WorkflowStep{
							{
								WorkflowStepBase: wfTypesv1alpha1.WorkflowStepBase{
									Name:    "step1",
									Type:    "deploy",
									Timeout: "invalid",
								},
							},
						},
					},
				},
			},
			expectedErrorCount: 1,
			expectedErrorField: "spec.workflow.steps.timeout",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			handler := &ValidatingHandler{
				Client: fake.NewClientBuilder().WithScheme(scheme).Build(),
			}

			req := admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					UserInfo: authenticationv1.UserInfo{
						Username: "test-user",
						Groups:   []string{"test-group"},
					},
				},
			}

			errs := handler.ValidateCreate(context.Background(), tc.app, req)
			assert.Equal(t, tc.expectedErrorCount, len(errs),
				"Expected %d errors, got %d: %v", tc.expectedErrorCount, len(errs), errs)

			if tc.expectedErrorField != "" && len(errs) > 0 {
				assert.Contains(t, errs[0].Field, tc.expectedErrorField,
					"Expected error field to contain %s, got %s", tc.expectedErrorField, errs[0].Field)
			}
		})
	}
}

func TestValidateUpdate(t *testing.T) {
	// Disable the definition validation feature for this test
	featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultMutableFeatureGate, features.ValidateDefinitionPermissions, false)

	// Enable sharding to skip component validation (since we don't have component definitions in tests)
	oldSharding := sharding.EnableSharding
	sharding.EnableSharding = true
	defer func() { sharding.EnableSharding = oldSharding }()

	scheme := runtime.NewScheme()
	_ = v1beta1.AddToScheme(scheme)
	_ = authv1.AddToScheme(scheme)

	handler := &ValidatingHandler{
		Client: fake.NewClientBuilder().WithScheme(scheme).Build(),
	}

	oldApp := &v1beta1.Application{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-app",
			Namespace: "default",
		},
		Spec: v1beta1.ApplicationSpec{
			Components: []common.ApplicationComponent{
				{
					Name: "comp1",
					Type: "webservice",
				},
			},
		},
	}

	testCases := []struct {
		name               string
		newApp             *v1beta1.Application
		expectedErrorCount int
	}{
		{
			name: "valid update",
			newApp: &v1beta1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-app",
					Namespace: "default",
				},
				Spec: v1beta1.ApplicationSpec{
					Components: []common.ApplicationComponent{
						{
							Name: "comp1",
							Type: "webservice",
						},
						{
							Name: "comp2",
							Type: "worker",
						},
					},
				},
			},
			expectedErrorCount: 0,
		},
		{
			name: "update with invalid annotations",
			newApp: &v1beta1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-app",
					Namespace: "default",
					Annotations: map[string]string{
						oam.AnnotationAutoUpdate:     "true",
						oam.AnnotationPublishVersion: "v1.0.0",
					},
				},
				Spec: v1beta1.ApplicationSpec{
					Components: []common.ApplicationComponent{
						{
							Name: "comp1",
							Type: "webservice",
						},
					},
				},
			},
			expectedErrorCount: 1,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					UserInfo: authenticationv1.UserInfo{
						Username: "test-user",
						Groups:   []string{"test-group"},
					},
				},
			}

			errs := handler.ValidateUpdate(context.Background(), tc.newApp, oldApp, req)
			assert.Equal(t, tc.expectedErrorCount, len(errs),
				"Expected %d errors, got %d: %v", tc.expectedErrorCount, len(errs), errs)
		})
	}
}

func TestValidateTimeout(t *testing.T) {
	handler := &ValidatingHandler{}

	testCases := []struct {
		name          string
		timeout       string
		expectedError bool
	}{
		{
			name:          "valid duration - seconds",
			timeout:       "30s",
			expectedError: false,
		},
		{
			name:          "valid duration - minutes",
			timeout:       "5m",
			expectedError: false,
		},
		{
			name:          "valid duration - hours",
			timeout:       "2h",
			expectedError: false,
		},
		{
			name:          "valid duration - complex",
			timeout:       "1h30m45s",
			expectedError: false,
		},
		{
			name:          "invalid duration",
			timeout:       "invalid",
			expectedError: true,
		},
		{
			name:          "invalid duration - missing unit",
			timeout:       "30",
			expectedError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			errs := handler.ValidateTimeout("test-step", tc.timeout)
			if tc.expectedError {
				assert.NotEmpty(t, errs, "Expected validation error for timeout %s", tc.timeout)
			} else {
				assert.Empty(t, errs, "Expected no validation error for timeout %s", tc.timeout)
			}
		})
	}
}

func TestValidateAnnotations(t *testing.T) {
	handler := &ValidatingHandler{}

	testCases := []struct {
		name               string
		annotations        map[string]string
		expectedErrorCount int
	}{
		{
			name:               "no annotations",
			annotations:        nil,
			expectedErrorCount: 0,
		},
		{
			name: "only autoUpdate",
			annotations: map[string]string{
				oam.AnnotationAutoUpdate: "true",
			},
			expectedErrorCount: 0,
		},
		{
			name: "only publishVersion",
			annotations: map[string]string{
				oam.AnnotationPublishVersion: "v1.0.0",
			},
			expectedErrorCount: 0,
		},
		{
			name: "both autoUpdate and publishVersion",
			annotations: map[string]string{
				oam.AnnotationAutoUpdate:     "true",
				oam.AnnotationPublishVersion: "v1.0.0",
			},
			expectedErrorCount: 1,
		},
		{
			name: "autoUpdate false with publishVersion",
			annotations: map[string]string{
				oam.AnnotationAutoUpdate:     "false",
				oam.AnnotationPublishVersion: "v1.0.0",
			},
			expectedErrorCount: 0,
		},
		{
			name: "autoUpdate true with empty publishVersion",
			annotations: map[string]string{
				oam.AnnotationAutoUpdate:     "true",
				oam.AnnotationPublishVersion: "",
			},
			expectedErrorCount: 0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			app := &v1beta1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-app",
					Namespace:   "default",
					Annotations: tc.annotations,
				},
			}

			errs := handler.ValidateAnnotations(context.Background(), app)
			assert.Equal(t, tc.expectedErrorCount, len(errs),
				"Expected %d errors, got %d: %v", tc.expectedErrorCount, len(errs), errs)
		})
	}
}

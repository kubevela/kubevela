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
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	admissionv1 "k8s.io/api/admission/v1"
	authenticationv1 "k8s.io/api/authentication/v1"
	authv1 "k8s.io/api/authorization/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	featuregatetesting "k8s.io/component-base/featuregate/testing"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	workflowv1alpha1 "github.com/kubevela/workflow/api/v1alpha1"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/auth"
	"github.com/oam-dev/kubevela/pkg/features"
	"github.com/oam-dev/kubevela/pkg/oam"
)

func TestValidateDefinitionPermissions(t *testing.T) {
	// Enable the authentication features for testing
	featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultMutableFeatureGate, features.ValidateDefinitionPermissions, true)
	oldAuthWithUser := auth.AuthenticationWithUser
	auth.AuthenticationWithUser = true
	defer func() { auth.AuthenticationWithUser = oldAuthWithUser }()

	testCases := []struct {
		name                string
		app                 *v1beta1.Application
		userInfo            authenticationv1.UserInfo
		allowedDefinitions  map[string]bool // resource/namespace/name -> allowed
		existingDefinitions map[string]bool // namespace/name -> exists
		expectedErrorCount  int
		expectedErrorFields []string
		expectedErrorMsgs   []string
	}{
		{
			name: "user has all permissions",
			app: &v1beta1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-app",
					Namespace: "test-ns",
				},
				Spec: v1beta1.ApplicationSpec{
					Components: []common.ApplicationComponent{
						{
							Name: "comp1",
							Type: "webservice",
							Traits: []common.ApplicationTrait{
								{Type: "scaler"},
							},
						},
					},
					Policies: []v1beta1.AppPolicy{
						{Name: "policy1", Type: "topology"},
					},
					Workflow: &v1beta1.Workflow{
						Steps: []workflowv1alpha1.WorkflowStep{
							{
								WorkflowStepBase: workflowv1alpha1.WorkflowStepBase{
									Name: "step1",
									Type: "deploy",
								},
							},
						},
					},
				},
			},
			userInfo: authenticationv1.UserInfo{
				Username: "test-user",
				Groups:   []string{"test-group"},
			},
			allowedDefinitions: map[string]bool{
				"componentdefinitions/vela-system/webservice": true,
				"traitdefinitions/vela-system/scaler":         true,
				"policydefinitions/vela-system/topology":      true,
				"workflowstepdefinitions/vela-system/deploy":  true,
			},
			existingDefinitions: map[string]bool{
				"vela-system/webservice": true,
				"vela-system/scaler":     true,
				"vela-system/topology":   true,
				"vela-system/deploy":     true,
			},
			expectedErrorCount: 0,
		},
		{
			name: "user lacks ComponentDefinition permission",
			app: &v1beta1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-app",
					Namespace: "test-ns",
				},
				Spec: v1beta1.ApplicationSpec{
					Components: []common.ApplicationComponent{
						{
							Name: "comp1",
							Type: "webservice",
						},
						{
							Name: "comp2",
							Type: "webservice", // Same type, should get two errors
						},
					},
				},
			},
			userInfo: authenticationv1.UserInfo{
				Username: "restricted-user",
				Groups:   []string{"restricted-group"},
			},
			allowedDefinitions: map[string]bool{
				"componentdefinitions/vela-system/webservice": false,
				"componentdefinitions/test-ns/webservice":     false,
			},
			expectedErrorCount:  2, // One for each component
			expectedErrorFields: []string{"spec.components[0].type", "spec.components[1].type"},
			expectedErrorMsgs:   []string{"cannot get ComponentDefinition \"webservice\""},
		},
		{
			name: "user lacks TraitDefinition permission",
			app: &v1beta1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-app",
					Namespace: "test-ns",
				},
				Spec: v1beta1.ApplicationSpec{
					Components: []common.ApplicationComponent{
						{
							Name: "comp1",
							Type: "webservice",
							Traits: []common.ApplicationTrait{
								{Type: "scaler"},
								{Type: "gateway"},
							},
						},
					},
				},
			},
			userInfo: authenticationv1.UserInfo{
				Username: "restricted-user",
				Groups:   []string{"restricted-group"},
			},
			allowedDefinitions: map[string]bool{
				"componentdefinitions/vela-system/webservice": true,
				"traitdefinitions/vela-system/scaler":         false,
				"traitdefinitions/test-ns/scaler":             false,
				"traitdefinitions/vela-system/gateway":        false,
				"traitdefinitions/test-ns/gateway":            false,
			},
			existingDefinitions: map[string]bool{
				"vela-system/webservice": true,
			},
			expectedErrorCount:  2,
			expectedErrorFields: []string{"spec.components[0].traits[1].type", "spec.components[0].traits[0].type"},
			expectedErrorMsgs:   []string{"cannot get TraitDefinition \"gateway\"", "cannot get TraitDefinition \"scaler\""},
		},
		{
			name: "user lacks PolicyDefinition permission",
			app: &v1beta1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-app",
					Namespace: "test-ns",
				},
				Spec: v1beta1.ApplicationSpec{
					Policies: []v1beta1.AppPolicy{
						{
							Name: "topology",
							Type: "topology",
							Properties: &runtime.RawExtension{
								Raw: []byte(`{"clusters":["local","remote"]}`),
							},
						},
						{
							Name: "override",
							Type: "override",
							Properties: &runtime.RawExtension{
								Raw: []byte(`{"components":[{"name":"comp1"}]}`),
							},
						},
					},
				},
			},
			userInfo: authenticationv1.UserInfo{
				Username: "policy-user",
				Groups:   []string{"policy-group"},
			},
			allowedDefinitions: map[string]bool{
				"policydefinitions/vela-system/topology": true,
				"policydefinitions/test-ns/topology":     true,
				"policydefinitions/vela-system/override": false,
				"policydefinitions/test-ns/override":     false,
			},
			existingDefinitions: map[string]bool{
				"vela-system/topology": true,
				"test-ns/topology":     true,
			},
			expectedErrorCount:  1,
			expectedErrorFields: []string{"spec.policies[1].type"},
			expectedErrorMsgs:   []string{"cannot get PolicyDefinition \"override\""},
		},
		{
			name: "empty application",
			app: &v1beta1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "empty-app",
					Namespace: "test-ns",
				},
				Spec: v1beta1.ApplicationSpec{},
			},
			userInfo: authenticationv1.UserInfo{
				Username: "empty-user",
				Groups:   []string{"empty-group"},
			},
			allowedDefinitions: map[string]bool{},
			expectedErrorCount: 0,
		},
		{
			name: "nil workflow",
			app: &v1beta1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "nil-workflow-app",
					Namespace: "test-ns",
				},
				Spec: v1beta1.ApplicationSpec{
					Components: []common.ApplicationComponent{
						{
							Name: "comp1",
							Type: "webservice",
						},
					},
					Workflow: nil,
				},
			},
			userInfo: authenticationv1.UserInfo{
				Username: "test-user",
				Groups:   []string{"test-group"},
			},
			allowedDefinitions: map[string]bool{
				"componentdefinitions/vela-system/webservice": true,
				"componentdefinitions/test-ns/webservice":     true,
			},
			existingDefinitions: map[string]bool{
				"vela-system/webservice": true,
				"test-ns/webservice":     true,
			},
			expectedErrorCount: 0,
		},
		{
			name: "mixed permissions",
			app: &v1beta1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "mixed-app",
					Namespace: "test-ns",
				},
				Spec: v1beta1.ApplicationSpec{
					Components: []common.ApplicationComponent{
						{
							Name: "allowed-comp",
							Type: "webservice",
							Traits: []common.ApplicationTrait{
								{Type: "scaler"},
								{Type: "ingress"},
							},
						},
						{
							Name: "denied-comp",
							Type: "worker",
						},
					},
					Policies: []v1beta1.AppPolicy{
						{
							Name: "topology",
							Type: "topology",
						},
						{
							Name: "garbage-collect",
							Type: "garbage-collect",
						},
					},
					Workflow: &v1beta1.Workflow{
						Steps: []workflowv1alpha1.WorkflowStep{
							{
								WorkflowStepBase: workflowv1alpha1.WorkflowStepBase{
									Name: "deploy",
									Type: "deploy",
								},
							},
							{
								WorkflowStepBase: workflowv1alpha1.WorkflowStepBase{
									Name: "notify",
									Type: "notification",
								},
							},
						},
					},
				},
			},
			userInfo: authenticationv1.UserInfo{
				Username: "mixed-user",
				Groups:   []string{"mixed-group"},
			},
			allowedDefinitions: map[string]bool{
				"componentdefinitions/vela-system/webservice":      true,
				"componentdefinitions/test-ns/webservice":          true,
				"componentdefinitions/vela-system/worker":          false,
				"componentdefinitions/test-ns/worker":              false,
				"traitdefinitions/vela-system/scaler":              false,
				"traitdefinitions/test-ns/scaler":                  false,
				"traitdefinitions/vela-system/ingress":             true,
				"traitdefinitions/test-ns/ingress":                 true,
				"policydefinitions/vela-system/topology":           true,
				"policydefinitions/test-ns/topology":               true,
				"policydefinitions/vela-system/garbage-collect":    false,
				"policydefinitions/test-ns/garbage-collect":        false,
				"workflowstepdefinitions/vela-system/deploy":       true,
				"workflowstepdefinitions/test-ns/deploy":           true,
				"workflowstepdefinitions/vela-system/notification": false,
				"workflowstepdefinitions/test-ns/notification":     false,
			},
			existingDefinitions: map[string]bool{
				"vela-system/webservice": true,
				"vela-system/ingress":    true,
				"vela-system/topology":   true,
				"vela-system/deploy":     true,
			},
			expectedErrorCount: 4,
			expectedErrorFields: []string{
				"spec.components[0].traits[0].type",
				"spec.components[1].type",
				"spec.policies[1].type",
				"spec.workflow.steps[1].type",
			},
			expectedErrorMsgs: []string{
				"cannot get TraitDefinition \"scaler\"",
				"cannot get ComponentDefinition \"worker\"",
				"cannot get PolicyDefinition \"garbage-collect\"",
				"cannot get WorkflowStepDefinition \"notification\"",
			},
		},
		{
			name: "SAR API failure returns error",
			app: &v1beta1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-app",
					Namespace: "test-ns",
				},
				Spec: v1beta1.ApplicationSpec{
					Components: []common.ApplicationComponent{
						{
							Name: "comp1",
							Type: "error-trigger",
						},
					},
				},
			},
			userInfo: authenticationv1.UserInfo{
				Username: "test-user",
				Groups:   []string{"test-group"},
			},
			allowedDefinitions: map[string]bool{
				"componentdefinitions/vela-system/error-trigger": false,
				"componentdefinitions/test-ns/error-trigger":     false,
			},
			expectedErrorCount:  1,
			expectedErrorFields: []string{"spec.components[0].type"},
			expectedErrorMsgs:   []string{"unable to verify permissions"},
		},
		{
			name: "SAR API intermittent failure - fails closed for safety",
			app: &v1beta1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-app",
					Namespace: "test-ns",
				},
				Spec: v1beta1.ApplicationSpec{
					Components: []common.ApplicationComponent{
						{
							Name: "comp1",
							Type: "system-error-only",
						},
					},
				},
			},
			userInfo: authenticationv1.UserInfo{
				Username: "test-user",
				Groups:   []string{"test-group"},
			},
			allowedDefinitions: map[string]bool{
				"componentdefinitions/test-ns/system-error-only": true,
			},
			expectedErrorCount:  1,
			expectedErrorFields: []string{"spec.components[0].type"},
			expectedErrorMsgs:   []string{"unable to verify permissions"},
		},
		{
			name: "user has permission in app namespace but not system namespace",
			app: &v1beta1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-app",
					Namespace: "test-ns",
				},
				Spec: v1beta1.ApplicationSpec{
					Components: []common.ApplicationComponent{
						{
							Name: "comp1",
							Type: "custom-comp",
						},
					},
				},
			},
			userInfo: authenticationv1.UserInfo{
				Username: "namespace-user",
				Groups:   []string{"namespace-group"},
			},
			allowedDefinitions: map[string]bool{
				"componentdefinitions/vela-system/custom-comp": false,
				"componentdefinitions/test-ns/custom-comp":     true, // Allowed in app namespace
			},
			existingDefinitions: map[string]bool{
				// Definition exists in app namespace
				"test-ns/custom-comp": true,
			},
			expectedErrorCount: 0, // Should pass as user has permission in their namespace
		},
		{
			name: "user has permission in system namespace but not app namespace",
			app: &v1beta1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-app",
					Namespace: "test-ns",
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
			userInfo: authenticationv1.UserInfo{
				Username: "system-user",
				Groups:   []string{"system-group"},
			},
			allowedDefinitions: map[string]bool{
				"componentdefinitions/vela-system/webservice": true, // Allowed in system namespace
				"componentdefinitions/test-ns/webservice":     false,
			},
			existingDefinitions: map[string]bool{
				"vela-system/webservice": true,
			},
			expectedErrorCount: 0, // Should pass as user has permission in system namespace
		},
		{
			name: "workflow with substeps",
			app: &v1beta1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-app",
					Namespace: "test-ns",
				},
				Spec: v1beta1.ApplicationSpec{
					Workflow: &v1beta1.Workflow{
						Steps: []workflowv1alpha1.WorkflowStep{
							{
								WorkflowStepBase: workflowv1alpha1.WorkflowStepBase{
									Name: "step1",
									Type: "deploy",
								},
								SubSteps: []workflowv1alpha1.WorkflowStepBase{
									{
										Name: "substep1",
										Type: "suspend",
									},
									{
										Name: "substep2",
										Type: "notification",
									},
								},
							},
						},
					},
				},
			},
			userInfo: authenticationv1.UserInfo{
				Username: "workflow-user",
				Groups:   []string{"workflow-group"},
			},
			allowedDefinitions: map[string]bool{
				"workflowstepdefinitions/vela-system/deploy":       true,
				"workflowstepdefinitions/vela-system/suspend":      false,
				"workflowstepdefinitions/test-ns/suspend":          false,
				"workflowstepdefinitions/vela-system/notification": false,
				"workflowstepdefinitions/test-ns/notification":     false,
			},
			existingDefinitions: map[string]bool{
				"vela-system/deploy": true,
			},
			expectedErrorCount:  2,
			expectedErrorFields: []string{"spec.workflow.steps[0].subSteps[0].type", "spec.workflow.steps[0].subSteps[1].type"},
			expectedErrorMsgs:   []string{"cannot get WorkflowStepDefinition \"suspend\"", "cannot get WorkflowStepDefinition \"notification\""},
		},
		{
			name: "duplicate definitions only checked once",
			app: &v1beta1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-app",
					Namespace: "test-ns",
				},
				Spec: v1beta1.ApplicationSpec{
					Components: []common.ApplicationComponent{
						{
							Name: "comp1",
							Type: "webservice",
							Traits: []common.ApplicationTrait{
								{Type: "scaler"},
							},
						},
						{
							Name: "comp2",
							Type: "webservice", // Duplicate
							Traits: []common.ApplicationTrait{
								{Type: "scaler"}, // Duplicate
							},
						},
					},
				},
			},
			userInfo: authenticationv1.UserInfo{
				Username: "test-user",
				Groups:   []string{"test-group"},
			},
			allowedDefinitions: map[string]bool{
				"componentdefinitions/vela-system/webservice": false,
				"componentdefinitions/test-ns/webservice":     false,
				"traitdefinitions/vela-system/scaler":         false,
				"traitdefinitions/test-ns/scaler":             false,
			},
			expectedErrorCount: 4, // 2 components + 2 traits
			expectedErrorFields: []string{
				"spec.components[0].type",
				"spec.components[1].type",
				"spec.components[0].traits[0].type",
				"spec.components[1].traits[0].type",
			},
		},
		{
			name: "application in vela-system namespace",
			app: &v1beta1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-app",
					Namespace: oam.SystemDefinitionNamespace, // vela-system
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
			userInfo: authenticationv1.UserInfo{
				Username: "system-user",
				Groups:   []string{"system-group"},
			},
			allowedDefinitions: map[string]bool{
				"componentdefinitions/vela-system/webservice": false,
			},
			expectedErrorCount:  1,
			expectedErrorFields: []string{"spec.components[0].type"},
		},
		{
			name: "namespace admin cannot use vela-system definitions without explicit access",
			app: &v1beta1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-app",
					Namespace: "test",
				},
				Spec: v1beta1.ApplicationSpec{
					Components: []common.ApplicationComponent{
						{
							Name: "hello",
							Type: "hello-cm", // This definition exists only in vela-system
						},
					},
				},
			},
			userInfo: authenticationv1.UserInfo{
				Username: "system:serviceaccount:test:app-writer",
				Groups:   []string{"system:serviceaccounts", "system:serviceaccounts:test"},
			},
			allowedDefinitions: map[string]bool{
				// User has wildcard permissions in test namespace
				"componentdefinitions/test/hello-cm": true,
				// But no explicit access to vela-system
				"componentdefinitions/vela-system/hello-cm": false,
			},
			existingDefinitions: map[string]bool{
				// Definition exists in vela-system but not in test namespace
				"vela-system/hello-cm": true,
				"test/hello-cm":        false,
			},
			expectedErrorCount:  1,
			expectedErrorFields: []string{"spec.components[0].type"},
			expectedErrorMsgs:   []string{"cannot get ComponentDefinition \"hello-cm\""},
		},
		{
			name: "user has vela-system permission but definition does not exist there",
			app: &v1beta1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-app",
					Namespace: "test",
				},
				Spec: v1beta1.ApplicationSpec{
					Components: []common.ApplicationComponent{
						{
							Name: "phantom",
							Type: "phantom-def", // User has permission but doesn't exist
						},
					},
				},
			},
			userInfo: authenticationv1.UserInfo{
				Username: "phantom-user",
				Groups:   []string{"phantom-group"},
			},
			allowedDefinitions: map[string]bool{
				// User has explicit permission to phantom-def in vela-system
				"componentdefinitions/vela-system/phantom-def": true,
				// And also in test namespace
				"componentdefinitions/test/phantom-def": true,
			},
			existingDefinitions: map[string]bool{
				// But definition doesn't exist in either namespace
				"vela-system/phantom-def": false,
				"test/phantom-def":        false,
			},
			expectedErrorCount:  1,
			expectedErrorFields: []string{"spec.components[0].type"},
			expectedErrorMsgs:   []string{"cannot get ComponentDefinition \"phantom-def\""},
		},
		{
			name: "user has vela-system permission but definition only exists in app namespace",
			app: &v1beta1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-app",
					Namespace: "test",
				},
				Spec: v1beta1.ApplicationSpec{
					Components: []common.ApplicationComponent{
						{
							Name: "local-only",
							Type: "local-only-def", // Exists only in app namespace
						},
					},
				},
			},
			userInfo: authenticationv1.UserInfo{
				Username: "mixed-user",
				Groups:   []string{"mixed-group"},
			},
			allowedDefinitions: map[string]bool{
				// User has permission in both namespaces
				"componentdefinitions/vela-system/local-only-def": true,
				"componentdefinitions/test/local-only-def":        true,
			},
			existingDefinitions: map[string]bool{
				// Definition only exists in app namespace
				"vela-system/local-only-def": false,
				"test/local-only-def":        true,
			},
			expectedErrorCount: 0, // Should succeed using test namespace version
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create a fake client with mock SubjectAccessReview behavior
			scheme := runtime.NewScheme()
			_ = v1beta1.AddToScheme(scheme)
			_ = authv1.AddToScheme(scheme)

			fakeClient := &mockSARClient{
				Client:              fake.NewClientBuilder().WithScheme(scheme).Build(),
				allowedDefinitions:  tc.allowedDefinitions,
				existingDefinitions: tc.existingDefinitions,
			}

			handler := &ValidatingHandler{
				Client: fakeClient,
			}

			req := admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					UserInfo: tc.userInfo,
				},
			}

			// Run the validation
			errs := handler.ValidateDefinitionPermissions(context.Background(), tc.app, req)

			// Check error count
			assert.Equal(t, tc.expectedErrorCount, len(errs),
				"Expected %d errors, got %d: %v", tc.expectedErrorCount, len(errs), errs)

			// Check error fields and messages (order independent)
			if len(tc.expectedErrorFields) > 0 {
				actualFields := make([]string, len(errs))
				for i, err := range errs {
					actualFields[i] = err.Field
				}
				for _, expectedField := range tc.expectedErrorFields {
					assert.Contains(t, actualFields, expectedField,
						"Expected field %s not found in errors: %v", expectedField, actualFields)
				}
			}
			if len(tc.expectedErrorMsgs) > 0 {
				actualMessages := make([]string, len(errs))
				for i, err := range errs {
					actualMessages[i] = err.Detail
				}
				for _, expectedMsg := range tc.expectedErrorMsgs {
					found := false
					for _, actualMsg := range actualMessages {
						if strings.Contains(actualMsg, expectedMsg) {
							found = true
							break
						}
					}
					assert.True(t, found,
						"Expected message containing %s not found in errors: %v", expectedMsg, actualMessages)
				}
			}
		})
	}
}

func TestValidateDefinitionPermissions_FeatureDisabled(t *testing.T) {
	// Disable the definition validation feature
	featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultMutableFeatureGate, features.ValidateDefinitionPermissions, false)
	oldAuthWithUser := auth.AuthenticationWithUser
	auth.AuthenticationWithUser = true
	defer func() { auth.AuthenticationWithUser = oldAuthWithUser }()

	app := &v1beta1.Application{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-app",
			Namespace: "test-ns",
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

	handler := &ValidatingHandler{
		Client: fake.NewClientBuilder().Build(),
	}

	req := admission.Request{
		AdmissionRequest: admissionv1.AdmissionRequest{
			UserInfo: authenticationv1.UserInfo{
				Username: "test-user",
			},
		},
	}

	// Should return no errors when feature is disabled
	errs := handler.ValidateDefinitionPermissions(context.Background(), app, req)
	assert.Empty(t, errs, "Expected no errors when feature is disabled")
}

func TestValidateDefinitionPermissions_AuthenticationDisabled(t *testing.T) {
	// Enable the definition validation feature but disable authentication
	featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultMutableFeatureGate, features.ValidateDefinitionPermissions, true)
	oldAuthWithUser := auth.AuthenticationWithUser
	auth.AuthenticationWithUser = false
	defer func() { auth.AuthenticationWithUser = oldAuthWithUser }()

	app := &v1beta1.Application{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-app",
			Namespace: "test-ns",
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

	// Use mock client that allows the definition
	scheme := runtime.NewScheme()
	_ = v1beta1.AddToScheme(scheme)
	_ = authv1.AddToScheme(scheme)

	fakeClient := &mockSARClient{
		Client: fake.NewClientBuilder().WithScheme(scheme).Build(),
		allowedDefinitions: map[string]bool{
			"componentdefinitions/vela-system/webservice": true,
			"componentdefinitions/test-ns/webservice":     true,
		},
		existingDefinitions: map[string]bool{
			"vela-system/webservice": true,
			"test-ns/webservice":     true,
		},
	}

	handler := &ValidatingHandler{
		Client: fakeClient,
	}

	req := admission.Request{
		AdmissionRequest: admissionv1.AdmissionRequest{
			UserInfo: authenticationv1.UserInfo{
				Username: "test-user",
			},
		},
	}

	// Definition validation should still work even when AuthenticationWithUser is disabled
	// It validates based on the user info in the request regardless of auth flag
	errs := handler.ValidateDefinitionPermissions(context.Background(), app, req)
	assert.Empty(t, errs, "Expected no errors when user has permission even with AuthenticationWithUser disabled")
}

func TestCollectDefinitionUsage(t *testing.T) {
	app := &v1beta1.Application{
		Spec: v1beta1.ApplicationSpec{
			Components: []common.ApplicationComponent{
				{
					Name: "comp1",
					Type: "webservice",
					Traits: []common.ApplicationTrait{
						{Type: "scaler"},
						{Type: "gateway"},
					},
				},
				{
					Name: "comp2",
					Type: "webservice", // Duplicate
					Traits: []common.ApplicationTrait{
						{Type: "scaler"}, // Duplicate
					},
				},
			},
			Policies: []v1beta1.AppPolicy{
				{Name: "policy1", Type: "topology"},
				{Name: "policy2", Type: "override"},
				{Name: "policy3", Type: "topology"}, // Duplicate
			},
			Workflow: &v1beta1.Workflow{
				Steps: []workflowv1alpha1.WorkflowStep{
					{
						WorkflowStepBase: workflowv1alpha1.WorkflowStepBase{
							Name: "step1",
							Type: "deploy",
						},
						SubSteps: []workflowv1alpha1.WorkflowStepBase{
							{
								Name: "substep1",
								Type: "suspend",
							},
							{
								Name: "substep2",
								Type: "deploy", // Duplicate
							},
						},
					},
				},
			},
		},
	}

	usage := collectDefinitionUsage(app)

	// Check component types
	assert.Equal(t, 2, len(usage.componentTypes["webservice"]))
	assert.Contains(t, usage.componentTypes["webservice"], 0)
	assert.Contains(t, usage.componentTypes["webservice"], 1)

	// Check trait types
	assert.Equal(t, 2, len(usage.traitTypes["scaler"]))
	assert.Equal(t, 1, len(usage.traitTypes["gateway"]))

	// Check policy types
	assert.Equal(t, 2, len(usage.policyTypes["topology"]))
	assert.Equal(t, 1, len(usage.policyTypes["override"]))

	// Check workflow step types
	assert.Equal(t, 2, len(usage.workflowStepTypes["deploy"]))
	assert.Equal(t, 1, len(usage.workflowStepTypes["suspend"]))

	// Check that substeps are properly marked
	for _, loc := range usage.workflowStepTypes["suspend"] {
		assert.True(t, loc.IsSubStep, "Suspend should be marked as a substep")
		assert.Equal(t, 0, loc.StepIndex, "Suspend should be in step 0")
		assert.Equal(t, 0, loc.SubStepIndex, "Suspend should be substep 0")
	}

	// Check regular steps and substeps for "deploy"
	deployLocations := usage.workflowStepTypes["deploy"]
	assert.Equal(t, 2, len(deployLocations))
	regularStepFound := false
	subStepFound := false
	for _, loc := range deployLocations {
		if !loc.IsSubStep {
			regularStepFound = true
			assert.Equal(t, 0, loc.StepIndex)
			assert.Equal(t, -1, loc.SubStepIndex)
		} else {
			subStepFound = true
			assert.Equal(t, 0, loc.StepIndex)
			assert.Equal(t, 1, loc.SubStepIndex)
		}
	}
	assert.True(t, regularStepFound, "Should have regular deploy step")
	assert.True(t, subStepFound, "Should have deploy substep")
}

func TestGetWorkflowStepFieldPath(t *testing.T) {
	testCases := []struct {
		name         string
		location     workflowStepLocation
		expectedPath string
	}{
		{
			name: "regular step",
			location: workflowStepLocation{
				StepIndex:    0,
				SubStepIndex: -1,
				IsSubStep:    false,
			},
			expectedPath: "spec.workflow.steps[0].type",
		},
		{
			name: "regular step index 5",
			location: workflowStepLocation{
				StepIndex:    5,
				SubStepIndex: -1,
				IsSubStep:    false,
			},
			expectedPath: "spec.workflow.steps[5].type",
		},
		{
			name: "substep 0 of step 0",
			location: workflowStepLocation{
				StepIndex:    0,
				SubStepIndex: 0,
				IsSubStep:    true,
			},
			expectedPath: "spec.workflow.steps[0].subSteps[0].type",
		},
		{
			name: "substep 2 of step 1",
			location: workflowStepLocation{
				StepIndex:    1,
				SubStepIndex: 2,
				IsSubStep:    true,
			},
			expectedPath: "spec.workflow.steps[1].subSteps[2].type",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			path := getWorkflowStepFieldPath(tc.location)
			assert.Equal(t, tc.expectedPath, path.String())
		})
	}
}

// mockSARClient is a mock client that simulates SubjectAccessReview responses
type mockSARClient struct {
	client.Client
	allowedDefinitions  map[string]bool
	existingDefinitions map[string]bool // namespace/name -> exists
}

func (m *mockSARClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	if sar, ok := obj.(*authv1.SubjectAccessReview); ok {
		// Mock failures
		if sar.Spec.ResourceAttributes.Name == "error-trigger" {
			return fmt.Errorf("simulated SAR API failure: connection timeout")
		}
		if sar.Spec.ResourceAttributes.Name == "system-error-only" &&
			sar.Spec.ResourceAttributes.Namespace == "vela-system" {
			return fmt.Errorf("simulated SAR API failure: system namespace unreachable")
		}

		key := fmt.Sprintf("%s/%s/%s",
			sar.Spec.ResourceAttributes.Resource,
			sar.Spec.ResourceAttributes.Namespace,
			sar.Spec.ResourceAttributes.Name)

		if allowed, exists := m.allowedDefinitions[key]; exists {
			sar.Status.Allowed = allowed
		} else {
			sar.Status.Allowed = false
		}
		return nil
	}
	return m.Client.Create(ctx, obj, opts...)
}

func (m *mockSARClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	// Handle definition existence checks
	var resource string
	switch obj.(type) {
	case *v1beta1.ComponentDefinition:
		resource = "componentdefinitions"
	case *v1beta1.TraitDefinition:
		resource = "traitdefinitions"
	case *v1beta1.PolicyDefinition:
		resource = "policydefinitions"
	case *v1beta1.WorkflowStepDefinition:
		resource = "workflowstepdefinitions"
	default:
		return m.Client.Get(ctx, key, obj, opts...)
	}

	defKey := fmt.Sprintf("%s/%s", key.Namespace, key.Name)
	if m.existingDefinitions != nil {
		if exists, ok := m.existingDefinitions[defKey]; ok && exists {
			// Definition exists - return success
			return nil
		}
	}
	// Definition not found - use correct resource type in error
	return errors.NewNotFound(v1beta1.SchemeGroupVersion.WithResource(resource).GroupResource(), key.Name)
}

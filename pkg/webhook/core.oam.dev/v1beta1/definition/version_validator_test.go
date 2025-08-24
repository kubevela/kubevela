/*
Copyright 2023 The KubeVela Authors.

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

package definition

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/kubevela/kubevela/apis/core.oam.dev/v1beta1"
)

func TestVersionValidator(t *testing.T) {
	// Create fake client
	scheme := runtime.NewScheme()
	assert.NoError(t, v1beta1.AddToScheme(scheme))
	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	decoder, err := admission.NewDecoder(scheme)
	assert.NoError(t, err)

	validator := &VersionValidator{
		Client:  client,
		Decoder: decoder,
	}

	testCases := []struct {
		name          string
		operation     admissionv1.Operation
		oldObject     map[string]interface{}
		newObject     map[string]interface{}
		expectedAllow bool
	}{
		{
			name:      "Create operation - always allowed",
			operation: admissionv1.Create,
			oldObject: nil,
			newObject: map[string]interface{}{
				"apiVersion": "core.oam.dev/v1beta1",
				"kind":       "ComponentDefinition",
				"metadata": map[string]interface{}{
					"name":      "test-comp",
					"namespace": "default",
				},
				"spec": map[string]interface{}{
					"version": "1.0.0",
				},
			},
			expectedAllow: true,
		},
		{
			name:      "Update operation - same version - denied",
			operation: admissionv1.Update,
			oldObject: map[string]interface{}{
				"apiVersion": "core.oam.dev/v1beta1",
				"kind":       "ComponentDefinition",
				"metadata": map[string]interface{}{
					"name":      "test-comp",
					"namespace": "default",
				},
				"spec": map[string]interface{}{
					"version": "1.0.0",
				},
			},
			newObject: map[string]interface{}{
				"apiVersion": "core.oam.dev/v1beta1",
				"kind":       "ComponentDefinition",
				"metadata": map[string]interface{}{
					"name":      "test-comp",
					"namespace": "default",
				},
				"spec": map[string]interface{}{
					"version": "1.0.0", // Same version - should be denied
				},
			},
			expectedAllow: false,
		},
		{
			name:      "Update operation - different version - allowed",
			operation: admissionv1.Update,
			oldObject: map[string]interface{}{
				"apiVersion": "core.oam.dev/v1beta1",
				"kind":       "ComponentDefinition",
				"metadata": map[string]interface{}{
					"name":      "test-comp",
					"namespace": "default",
				},
				"spec": map[string]interface{}{
					"version": "1.0.0",
				},
			},
			newObject: map[string]interface{}{
				"apiVersion": "core.oam.dev/v1beta1",
				"kind":       "ComponentDefinition",
				"metadata": map[string]interface{}{
					"name":      "test-comp",
					"namespace": "default",
				},
				"spec": map[string]interface{}{
					"version": "1.1.0", // Different version - should be allowed
				},
			},
			expectedAllow: true,
		},
		{
			name:      "Update operation - no version - allowed",
			operation: admissionv1.Update,
			oldObject: map[string]interface{}{
				"apiVersion": "core.oam.dev/v1beta1",
				"kind":       "ComponentDefinition",
				"metadata": map[string]interface{}{
					"name":      "test-comp",
					"namespace": "default",
				},
				"spec": map[string]interface{}{},
			},
			newObject: map[string]interface{}{
				"apiVersion": "core.oam.dev/v1beta1",
				"kind":       "ComponentDefinition",
				"metadata": map[string]interface{}{
					"name":      "test-comp",
					"namespace": "default",
				},
				"spec": map[string]interface{}{},
			},
			expectedAllow: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Prepare request
			req := admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: tc.operation,
				},
			}

			// Marshal objects to JSON
			if tc.oldObject != nil {
				oldRaw, err := json.Marshal(tc.oldObject)
				assert.NoError(t, err)
				req.OldObject = runtime.RawExtension{Raw: oldRaw}
			}

			if tc.newObject != nil {
				newRaw, err := json.Marshal(tc.newObject)
				assert.NoError(t, err)
				req.Object = runtime.RawExtension{Raw: newRaw}
			}

			// Call the webhook
			resp := validator.Handle(context.Background(), req)

			// Check the result
			assert.Equal(t, tc.expectedAllow, resp.Allowed, "Response: %v", resp.Result)
		})
	}
}

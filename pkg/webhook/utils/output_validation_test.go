/*
 Copyright 2021. The KubeVela Authors.

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

package utils

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// mockRESTMapperForValidation is a mock implementation of meta.RESTMapper for testing output validation
type mockRESTMapperForValidation struct {
	existingGVKs map[schema.GroupVersionKind]bool
}

func (m *mockRESTMapperForValidation) KindFor(resource schema.GroupVersionResource) (schema.GroupVersionKind, error) {
	return schema.GroupVersionKind{}, nil
}

func (m *mockRESTMapperForValidation) KindsFor(resource schema.GroupVersionResource) ([]schema.GroupVersionKind, error) {
	return nil, nil
}

func (m *mockRESTMapperForValidation) ResourceFor(input schema.GroupVersionResource) (schema.GroupVersionResource, error) {
	return schema.GroupVersionResource{}, nil
}

func (m *mockRESTMapperForValidation) ResourcesFor(input schema.GroupVersionResource) ([]schema.GroupVersionResource, error) {
	return nil, nil
}

func (m *mockRESTMapperForValidation) RESTMapping(gk schema.GroupKind, versions ...string) (*meta.RESTMapping, error) {
	for _, version := range versions {
		gvk := schema.GroupVersionKind{
			Group:   gk.Group,
			Version: version,
			Kind:    gk.Kind,
		}
		if m.existingGVKs[gvk] {
			return &meta.RESTMapping{
				Resource: schema.GroupVersionResource{
					Group:    gk.Group,
					Version:  version,
					Resource: gk.Kind,
				},
				GroupVersionKind: gvk,
			}, nil
		}
	}
	return nil, &meta.NoKindMatchError{
		GroupKind:        gk,
		SearchedVersions: versions,
	}
}

func (m *mockRESTMapperForValidation) RESTMappings(gk schema.GroupKind, versions ...string) ([]*meta.RESTMapping, error) {
	return nil, nil
}

func (m *mockRESTMapperForValidation) ResourceSingularizer(resource string) (singular string, err error) {
	return "", nil
}

func TestValidateOutputResourcesExist(t *testing.T) {
	ctx := context.Background()
	
	// Create mock RESTMapper with some existing resources
	mapper := &mockRESTMapperForValidation{
		existingGVKs: map[schema.GroupVersionKind]bool{
			{Group: "apps", Version: "v1", Kind: "Deployment"}:    true,
			{Group: "", Version: "v1", Kind: "Service"}:           true,
			{Group: "", Version: "v1", Kind: "ConfigMap"}:         true,
			{Group: "batch", Version: "v1", Kind: "Job"}:          true,
		},
	}

	tests := []struct {
		name        string
		cueTemplate string
		wantErr     bool
		errContains string
	}{
		{
			name: "valid deployment in output",
			cueTemplate: `
output: {
	apiVersion: "apps/v1"
	kind: "Deployment"
	metadata: name: "test"
}`,
			wantErr: false,
		},
		{
			name: "valid service and configmap in outputs",
			cueTemplate: `
outputs: {
	svc: {
		apiVersion: "v1"
		kind: "Service"
		metadata: name: "test-svc"
	}
	cm: {
		apiVersion: "v1"
		kind: "ConfigMap"
		metadata: name: "test-cm"
	}
}`,
			wantErr: false,
		},
		{
			name: "invalid CRD in output",
			cueTemplate: `
output: {
	apiVersion: "custom.io/v1alpha1"
	kind: "CustomResource"
	metadata: name: "test"
}`,
			wantErr:     true,
			errContains: "does not exist on the cluster",
		},
		{
			name: "invalid CRD in outputs",
			cueTemplate: `
outputs: {
	valid: {
		apiVersion: "v1"
		kind: "Service"
		metadata: name: "test-svc"
	}
	invalid: {
		apiVersion: "unknown.io/v1"
		kind: "UnknownResource"
		metadata: name: "test-unknown"
	}
}`,
			wantErr:     true,
			errContains: "does not exist on the cluster",
		},
		{
			name: "output without apiVersion and kind (not a k8s resource)",
			cueTemplate: `
output: {
	someField: "value"
	anotherField: 123
}`,
			wantErr: false,
		},
		{
			name: "mixed valid and non-k8s resources in outputs",
			cueTemplate: `
outputs: {
	deployment: {
		apiVersion: "apps/v1"
		kind: "Deployment"
		metadata: name: "test"
	}
	customData: {
		field1: "value1"
		field2: "value2"
	}
}`,
			wantErr: false,
		},
		{
			name: "output with expression (should skip validation)",
			cueTemplate: `
import "strconv"

output: {
	apiVersion: "apps/v1"
	kind: "Deployment"
	metadata: name: "test-" + strconv.FormatInt(1, 10)
}`,
			wantErr: false,
		},
		{
			name: "empty template",
			cueTemplate: ``,
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateOutputResourcesExist(ctx, tt.cueTemplate, mapper)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
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

// Package utils contains unit tests for the output validation functionality.
//
// These tests validate the core logic of ValidateOutputResourcesExist function,
// which is responsible for checking that CUE templates in ComponentDefinitions,
// TraitDefinitions, and PolicyDefinitions only reference Kubernetes resources
// that exist on the cluster.
//
// The tests use a mock RESTMapper to simulate different cluster states and
// verify that validation behaves correctly for various scenarios including:
// - Valid Kubernetes resources (should pass)
// - Non-existent CRDs (should fail with specific error)
// - Non-Kubernetes objects (should be ignored)
// - Empty or malformed templates (should be handled gracefully)
// - Complex CUE expressions (should skip validation for dynamic content)
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
	// Create mock RESTMapper with some existing resources
	mapper := &mockRESTMapperForValidation{
		existingGVKs: map[schema.GroupVersionKind]bool{
			{Group: "apps", Version: "v1", Kind: "Deployment"}: true,
			{Group: "", Version: "v1", Kind: "Service"}:        true,
			{Group: "", Version: "v1", Kind: "ConfigMap"}:      true,
			{Group: "batch", Version: "v1", Kind: "Job"}:       true,
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
			name:        "empty template",
			cueTemplate: ``,
			wantErr:     false,
		},
		{
			name: "multiple invalid CRDs in outputs",
			cueTemplate: `
outputs: {
	valid: {
		apiVersion: "v1"
		kind: "Service"
		metadata: name: "test-svc"
	}
	invalid1: {
		apiVersion: "custom.io/v1alpha1"
		kind: "CustomResource"
		metadata: name: "test-custom1"
	}
	invalid2: {
		apiVersion: "another.io/v1beta1"
		kind: "AnotherResource"
		metadata: name: "test-custom2"
	}
}`,
			wantErr:     true,
			errContains: "does not exist on the cluster",
		},
		{
			name: "output with complex nested structure and invalid CRD",
			cueTemplate: `
output: {
	apiVersion: "custom.extension.io/v1"
	kind: "CustomWorkload"
	metadata: {
		name: "test-workload"
		labels: {
			app: "test"
			version: "v1"
		}
		annotations: {
			"custom.io/config": "enabled"
		}
	}
	spec: {
		replicas: 3
		template: {
			spec: {
				containers: [{
					name: "app"
					image: "nginx:latest"
				}]
			}
		}
	}
}`,
			wantErr:     true,
			errContains: "does not exist on the cluster",
		},
		{
			name: "outputs with CRD that has uppercase in group",
			cueTemplate: `
outputs: {
	customResource: {
		apiVersion: "Custom.IO/v1alpha1"
		kind: "UpperCaseGroupResource"
		metadata: name: "test-upper"
		spec: {
			enabled: true
		}
	}
}`,
			wantErr:     true,
			errContains: "does not exist on the cluster",
		},
		{
			name: "valid resources with different API versions",
			cueTemplate: `
outputs: {
	deployment: {
		apiVersion: "apps/v1"
		kind: "Deployment"
		metadata: name: "test-deployment"
	}
	job: {
		apiVersion: "batch/v1"
		kind: "Job"
		metadata: name: "test-job"
	}
	service: {
		apiVersion: "v1"
		kind: "Service"
		metadata: name: "test-service"
	}
}`,
			wantErr: false,
		},
		{
			name: "output with malformed apiVersion",
			cueTemplate: `
output: {
	apiVersion: "invalid-format"
	kind: "SomeResource"
	metadata: name: "test"
}`,
			wantErr:     true,
			errContains: "does not exist on the cluster",
		},
		{
			name: "mixed resources with some having missing fields",
			cueTemplate: `
outputs: {
	validService: {
		apiVersion: "v1"
		kind: "Service"
		metadata: name: "test-svc"
	}
	missingKind: {
		apiVersion: "v1"
		metadata: name: "no-kind"
	}
	missingApiVersion: {
		kind: "ConfigMap"
		metadata: name: "no-version"
	}
	invalidResource: {
		apiVersion: "unknown.io/v1"
		kind: "UnknownResource"
		metadata: name: "invalid"
	}
}`,
			wantErr:     true,
			errContains: "does not exist on the cluster",
		},
		{
			name: "empty outputs object",
			cueTemplate: `
output: {
	apiVersion: "apps/v1"
	kind: "Deployment"
	metadata: name: "test"
}

outputs: {}`,
			wantErr: false,
		},
		{
			name: "outputs with deeply nested non-k8s data",
			cueTemplate: `
outputs: {
	config: {
		database: {
			host: "localhost"
			port: 5432
			credentials: {
				username: "admin"
				password: "secret"
			}
		}
		features: {
			authentication: true
			logging: {
				level: "info"
				format: "json"
			}
		}
	}
	validResource: {
		apiVersion: "v1"
		kind: "ConfigMap"
		metadata: name: "app-config"
	}
}`,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateOutputResourcesExist(tt.cueTemplate, mapper)
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

func TestValidateOutputResourcesExistEdgeCases(t *testing.T) {
	ctx := context.Background()

	// Create mapper with only core resources
	mapper := &mockRESTMapperForValidation{
		existingGVKs: map[schema.GroupVersionKind]bool{
			{Group: "", Version: "v1", Kind: "Pod"}:       true,
			{Group: "", Version: "v1", Kind: "Service"}:   true,
			{Group: "", Version: "v1", Kind: "ConfigMap"}: true,
			{Group: "", Version: "v1", Kind: "Secret"}:    true,
			{Group: "", Version: "v1", Kind: "Namespace"}: true,
		},
	}

	tests := []struct {
		name        string
		cueTemplate string
		wantErr     bool
		errContains string
	}{
		{
			name:        "completely invalid CUE syntax",
			cueTemplate: `this is not valid CUE at all {{{`,
			wantErr:     true,
		},
		{
			name: "CUE with incomplete field references",
			cueTemplate: `
parameter: {
	name: string
}

output: {
	apiVersion: "v1"
	kind: "Pod"
	metadata: name: parameter.nonExistentField
}`,
			wantErr: false, // Should not fail validation since field reference errors are CUE evaluation errors
		},
		{
			name: "template with import statements and complex logic",
			cueTemplate: `
import "strings"
import "encoding/json"

parameter: {
	name: string
	data: {...}
}

output: {
	apiVersion: "v1"
	kind: "ConfigMap"
	metadata: {
		name: strings.ToLower(parameter.name)
		annotations: {
			"config.json": json.Marshal(parameter.data)
		}
	}
}

outputs: {
	secret: {
		apiVersion: "v1"  
		kind: "Secret"
		metadata: name: parameter.name + "-secret"
		data: {
			config: json.Marshal(parameter.data)
		}
	}
}`,
			wantErr: false,
		},
		{
			name: "template with conditional outputs",
			cueTemplate: `
parameter: {
	name: string
	enableService: bool | *false
}

output: {
	apiVersion: "v1"
	kind: "Pod"
	metadata: name: parameter.name
}

if parameter.enableService {
	outputs: service: {
		apiVersion: "v1"
		kind: "Service"
		metadata: name: parameter.name + "-svc"
	}
}`,
			wantErr: false,
		},
		{
			name: "template with list comprehension",
			cueTemplate: `
parameter: {
	name: string
	configs: [...string]
}

output: {
	apiVersion: "v1"
	kind: "Pod"
	metadata: name: parameter.name
}

outputs: {
	for i, config in parameter.configs {
		"config-\(i)": {
			apiVersion: "v1"
			kind: "ConfigMap"
			metadata: name: "\(parameter.name)-config-\(i)"
			data: config: config
		}
	}
}`,
			wantErr: false,
		},
		{
			name: "template with embedded resource that doesn't exist",
			cueTemplate: `
outputs: {
	embedded: {
		// This looks like a K8s resource but the CRD doesn't exist
		apiVersion: "embed.io/v1alpha1"
		kind: "EmbeddedResource"
		metadata: name: "test"
		spec: {
			content: {
				apiVersion: "v1"
				kind: "ConfigMap"
				metadata: name: "inner-config"
			}
		}
	}
}`,
			wantErr:     true,
			errContains: "does not exist on the cluster",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateOutputResourcesExist(tt.cueTemplate, mapper)
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

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

package v1beta1

import (
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestDefinitionTypeMap(t *testing.T) {
	tests := []struct {
		name         string
		defType      reflect.Type
		expectedGVR  schema.GroupVersionResource
		expectedKind string
	}{
		{
			name:         "ComponentDefinition",
			defType:      reflect.TypeOf(ComponentDefinition{}),
			expectedGVR:  ComponentDefinitionGVR,
			expectedKind: ComponentDefinitionKind,
		},
		{
			name:         "TraitDefinition",
			defType:      reflect.TypeOf(TraitDefinition{}),
			expectedGVR:  TraitDefinitionGVR,
			expectedKind: TraitDefinitionKind,
		},
		{
			name:         "PolicyDefinition",
			defType:      reflect.TypeOf(PolicyDefinition{}),
			expectedGVR:  PolicyDefinitionGVR,
			expectedKind: PolicyDefinitionKind,
		},
		{
			name:         "WorkflowStepDefinition",
			defType:      reflect.TypeOf(WorkflowStepDefinition{}),
			expectedGVR:  WorkflowStepDefinitionGVR,
			expectedKind: WorkflowStepDefinitionKind,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info, ok := DefinitionTypeMap[tt.defType]
			assert.Truef(t, ok, "Type %v should exist in DefinitionTypeMap", tt.defType)
			assert.Equal(t, tt.expectedGVR, info.GVR)
			assert.Equal(t, tt.expectedKind, info.Kind)

			// Verify GVR follows Kubernetes conventions
			assert.Equal(t, Group, info.GVR.Group)
			assert.Equal(t, Version, info.GVR.Version)
			// Resource should be lowercase plural of Kind
			assert.Equal(t, strings.ToLower(info.Kind)+"s", info.GVR.Resource)
		})
	}
}

func TestDefinitionTypeMapCompleteness(t *testing.T) {
	// Ensure all expected definition types are in the map
	expectedTypes := []reflect.Type{
		reflect.TypeOf(ComponentDefinition{}),
		reflect.TypeOf(TraitDefinition{}),
		reflect.TypeOf(PolicyDefinition{}),
		reflect.TypeOf(WorkflowStepDefinition{}),
	}

	assert.Equal(t, len(expectedTypes), len(DefinitionTypeMap), "DefinitionTypeMap should contain exactly %d entries", len(expectedTypes))

	for _, expectedType := range expectedTypes {
		_, ok := DefinitionTypeMap[expectedType]
		assert.Truef(t, ok, "DefinitionTypeMap should contain %v", expectedType)
	}
}

func TestDefinitionKindValues(t *testing.T) {
	// Verify that the Kind values match the actual type names
	tests := []struct {
		defType      interface{}
		expectedKind string
	}{
		{ComponentDefinition{}, "ComponentDefinition"},
		{TraitDefinition{}, "TraitDefinition"},
		{PolicyDefinition{}, "PolicyDefinition"},
		{WorkflowStepDefinition{}, "WorkflowStepDefinition"},
	}

	for _, tt := range tests {
		t.Run(tt.expectedKind, func(t *testing.T) {
			actualKind := reflect.TypeOf(tt.defType).Name()
			assert.Equal(t, tt.expectedKind, actualKind)

			// Also verify it matches what's in the map
			info, ok := DefinitionTypeMap[reflect.TypeOf(tt.defType)]
			assert.True(t, ok)
			assert.Equal(t, tt.expectedKind, info.Kind)
		})
	}
}

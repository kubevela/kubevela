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

package applicationconfiguration

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
)

func TestCheckTraitObj(t *testing.T) {
	traitWithName := unstructured.Unstructured{
		Object: make(map[string]interface{}),
	}
	unstructured.SetNestedField(traitWithName.Object, "test", TraitTypeField)

	traitWithProperties := unstructured.Unstructured{
		Object: make(map[string]interface{}),
	}
	unstructured.SetNestedField(traitWithProperties.Object, "test", TraitSpecField)

	traitWithoutGVK := unstructured.Unstructured{}
	traitWithoutGVK.SetAPIVersion("")
	traitWithoutGVK.SetKind("")

	tests := []struct {
		caseName     string
		traitContent unstructured.Unstructured
		want         string
	}{
		{
			caseName:     "the trait contains 'name' info that should be mutated to GVK",
			traitContent: traitWithName,
			want:         "the trait contains 'name' info",
		},
		{
			caseName:     "the trait contains 'properties' info that should be mutated to spec",
			traitContent: traitWithProperties,
			want:         "the trait contains 'properties' info",
		},
		{
			caseName:     "the trait data missing GVK",
			traitContent: traitWithoutGVK,
			want:         "the trait data missing GVK",
		},
	}

	for _, tc := range tests {
		result := checkTraitObj(&tc.traitContent)
		assert.Contains(t, result.Error(), tc.want, fmt.Sprintf("Test case: %q", tc.caseName))
	}
}

func TestCheckParams(t *testing.T) {
	wlNameValue := "wlName"
	pName := "wlnameParam"
	wlParam := v1alpha2.ComponentParameter{
		Name:       pName,
		FieldPaths: []string{WorkloadNamePath},
	}
	wlParamValue := v1alpha2.ComponentParameterValue{
		Name:  pName,
		Value: intstr.FromString(wlNameValue),
	}

	mockValue := "mockValue"
	mockPName := "mockParam"
	mockFieldPath := "a.b"
	mockParam := v1alpha2.ComponentParameter{
		Name:       mockPName,
		FieldPaths: []string{mockFieldPath},
	}
	mockParamValue := v1alpha2.ComponentParameterValue{
		Name:  pName,
		Value: intstr.FromString(mockValue),
	}
	tests := []struct {
		caseName         string
		cps              []v1alpha2.ComponentParameter
		cpvs             []v1alpha2.ComponentParameterValue
		expectResult     bool
		expectParamValue string
	}{
		{
			caseName:         "get wokload name params and value",
			cps:              []v1alpha2.ComponentParameter{wlParam},
			cpvs:             []v1alpha2.ComponentParameterValue{wlParamValue},
			expectResult:     false,
			expectParamValue: wlNameValue,
		},
		{
			caseName:         "not found workload name params",
			cps:              []v1alpha2.ComponentParameter{mockParam},
			cpvs:             []v1alpha2.ComponentParameterValue{mockParamValue},
			expectResult:     true,
			expectParamValue: "",
		},
	}

	for _, tc := range tests {
		func(t *testing.T) {
			result, pValue := checkParams(tc.cps, tc.cpvs)
			assert.Equal(t, result, tc.expectResult,
				fmt.Sprintf("test case: %v", tc.caseName))
			assert.Equal(t, pValue, tc.expectParamValue,
				fmt.Sprintf("test case: %v", tc.caseName))
		}(t)

	}
}

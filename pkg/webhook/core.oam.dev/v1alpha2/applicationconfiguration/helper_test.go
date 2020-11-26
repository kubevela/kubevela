package applicationconfiguration

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
)

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

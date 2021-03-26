package appfile

import (
	"testing"

	"github.com/crossplane/crossplane-runtime/pkg/test"
	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/utils/pointer"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
)

func TestResolveKubeParameters(t *testing.T) {
	stringParam := &common.KubeParameter{
		Name:       "strParam",
		ValueType:  common.StringType,
		FieldPaths: []string{"spec"},
	}
	requiredParam := &common.KubeParameter{
		Name:       "reqParam",
		Required:   pointer.BoolPtr(true),
		ValueType:  common.StringType,
		FieldPaths: []string{"spec"},
	}
	tests := map[string]struct {
		reason   string
		params   []common.KubeParameter
		settings map[string]interface{}
		want     paramValueSettings
		wantErr  error
	}{
		"EmptyParam": {
			reason: "Empty value settings and no error should be returned",
			want:   make(paramValueSettings),
		},
		"UnsupportedParam": {
			reason:   "An error shoulde be returned because of unsupported param",
			params:   []common.KubeParameter{*stringParam},
			settings: map[string]interface{}{"unsupported": "invalid parameter"},
			want:     nil,
			wantErr:  errors.Errorf("unsupported parameter %q", "unsupported"),
		},
		"MissingRequiredParam": {
			reason:   "An error should be returned because of missing required param",
			params:   []common.KubeParameter{*stringParam, *requiredParam},
			settings: map[string]interface{}{"strParam": "string"},
			want:     nil,
			wantErr:  errors.Errorf("require parameter %q", "reqParam"),
		},
		"Succeed": {
			reason:   "No error should be returned",
			params:   []common.KubeParameter{*stringParam, *requiredParam},
			settings: map[string]interface{}{"strParam": "test", "reqParam": "test"},
			want: paramValueSettings{
				"strParam": paramValueSetting{
					Value:      "test",
					ValueType:  common.StringType,
					FieldPaths: stringParam.FieldPaths,
				},
				"reqParam": paramValueSetting{
					Value:      "test",
					ValueType:  common.StringType,
					FieldPaths: requiredParam.FieldPaths,
				},
			},
			wantErr: nil,
		},
	}

	for tcName, tc := range tests {
		t.Run(tcName, func(t *testing.T) {
			result, err := resolveKubeParameters(tc.params, tc.settings)
			if diff := cmp.Diff(tc.want, result); diff != "" {
				t.Fatalf("\nresolveKubeParameters(...)(...) -want +get \nreason:%s\n%s\n", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.wantErr, err, test.EquateErrors()); diff != "" {
				t.Fatalf("\nresolveKubeParameters(...)(...) -want +get \nreason:%s\n%s\n", tc.reason, diff)
			}
		})
	}

}

func TestSetParameterValuesToKubeObj(t *testing.T) {
	tests := map[string]struct {
		reason  string
		obj     unstructured.Unstructured
		values  paramValueSettings
		wantObj unstructured.Unstructured
		wantErr error
	}{
		"InvalidStringType": {
			reason: "An error should be returned",
			values: paramValueSettings{
				"strParam": paramValueSetting{
					Value:      int32(100),
					ValueType:  common.StringType,
					FieldPaths: []string{"spec.test"},
				},
			},
			wantErr: errors.Errorf(errInvalidValueType, common.StringType),
		},
		"InvalidNumberType": {
			reason: "An error should be returned",
			values: paramValueSettings{
				"intParam": paramValueSetting{
					Value:      "test",
					ValueType:  common.NumberType,
					FieldPaths: []string{"spec.test"},
				},
			},
			wantErr: errors.Errorf(errInvalidValueType, common.NumberType),
		},
		"InvalidBoolType": {
			reason: "An error should be returned",
			values: paramValueSettings{
				"boolParam": paramValueSetting{
					Value:      "test",
					ValueType:  common.BooleanType,
					FieldPaths: []string{"spec.test"},
				},
			},
			wantErr: errors.Errorf(errInvalidValueType, common.BooleanType),
		},
		"InvalidFieldPath": {
			reason: "An error should be returned",
			values: paramValueSettings{
				"strParam": paramValueSetting{
					Value:      "test",
					ValueType:  common.StringType,
					FieldPaths: []string{"spec[.test"}, // a invalid field path
				},
			},
			wantErr: errors.Wrap(errors.New(`cannot parse path "spec[.test": unterminated '[' at position 4`),
				`cannot set parameter "strParam" to field "spec[.test"`),
		},
		"Succeed": {
			reason: "No error should be returned",
			obj:    unstructured.Unstructured{Object: make(map[string]interface{})},
			values: paramValueSettings{
				"strParam": paramValueSetting{
					Value:      "test",
					ValueType:  common.StringType,
					FieldPaths: []string{"spec.strField"},
				},
				"intParam": paramValueSetting{
					Value:      10,
					ValueType:  common.NumberType,
					FieldPaths: []string{"spec.intField"},
				},
				"floatParam": paramValueSetting{
					Value:      float64(10.01),
					ValueType:  common.NumberType,
					FieldPaths: []string{"spec.floatField"},
				},
				"boolParam": paramValueSetting{
					Value:      true,
					ValueType:  common.BooleanType,
					FieldPaths: []string{"spec.boolField"},
				},
			},
			wantObj: unstructured.Unstructured{Object: map[string]interface{}{
				"spec": map[string]interface{}{
					"strField":   "test",
					"intField":   int64(10),
					"floatField": float64(10.01),
					"boolField":  true,
				},
			}},
		},
	}

	for tcName, tc := range tests {
		t.Run(tcName, func(t *testing.T) {
			obj := tc.obj.DeepCopy()
			err := setParameterValuesToKubeObj(obj, tc.values)
			if diff := cmp.Diff(tc.wantObj, *obj); diff != "" {
				t.Errorf("\nsetParameterValuesToKubeObj(...)error -want +get \nreason:%s\n%s\n", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.wantErr, err, test.EquateErrors()); diff != "" {
				t.Errorf("\nsetParameterValuesToKubeObj(...)error -want +get \nreason:%s\n%s\n", tc.reason, diff)
			}
		})
	}

}

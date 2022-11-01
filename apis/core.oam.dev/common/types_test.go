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

package common

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestOAMObjectReference(t *testing.T) {
	r := require.New(t)
	o1 := OAMObjectReference{
		Component: "component",
		Trait:     "trait",
		Env:       "env",
	}
	obj := &unstructured.Unstructured{}
	o2 := NewOAMObjectReferenceFromObject(obj)
	r.False(o2.Equal(o1))
	o1.AddLabelsToObject(obj)
	r.Equal(3, len(obj.GetLabels()))
	o3 := NewOAMObjectReferenceFromObject(obj)
	r.True(o1.Equal(o3))
	o3.Component = "comp"
	r.False(o3.Equal(o1))

	r.True(o1.Equal(*o1.DeepCopy()))
	o4 := OAMObjectReference{}
	o1.DeepCopyInto(&o4)
	r.True(o4.Equal(o1))
}

func TestClusterObjectReference(t *testing.T) {
	r := require.New(t)
	o1 := ClusterObjectReference{
		Cluster:         "cluster",
		ObjectReference: v1.ObjectReference{Kind: "kind"},
	}
	o2 := *o1.DeepCopy()
	r.True(o1.Equal(o2))
	o2.Cluster = "c"
	r.False(o2.Equal(o1))
}

type mockObject struct {
	client.Object
	labels map[string]string
}

func (o *mockObject) GetLabels() map[string]string {
	return o.labels
}
func (o *mockObject) SetLabels(labels map[string]string) {
	o.labels = labels
}

type mockObject2 struct {
	client.Object
	labels map[string]string
}

func (o *mockObject2) GetLabels() map[string]string {
	return map[string]string{
		oam.LabelAppComponent: "default",
		oam.TraitTypeLabel:    "default",
		oam.LabelAppEnv:       "default",
	}
}

func TestOAMObjectReference_AddLabelsToObject(t *testing.T) {
	tests := []struct {
		name      string
		Component string
		Trait     string
		Env       string
		obj       client.Object
		want      map[string]string
	}{
		{
			name: "base",
			obj:  &mockObject{},
			want: map[string]string{},
		},
		{
			name:      "set component",
			Component: "default",
			obj:       &mockObject{},
			want: map[string]string{
				oam.LabelAppComponent: "default",
			},
		},
		{
			name:  "set trait",
			Trait: "default",
			obj:   &mockObject{},
			want: map[string]string{
				oam.TraitTypeLabel: "default",
			},
		},
		{
			name: "set env",
			Env:  "default",
			obj:  &mockObject{},
			want: map[string]string{
				oam.LabelAppEnv: "default",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := OAMObjectReference{
				Component: tt.Component,
				Trait:     tt.Trait,
				Env:       tt.Env,
			}
			in.AddLabelsToObject(tt.obj)
			if got := tt.obj.GetLabels(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("GetLabels() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewOAMObjectReferenceFromObject(t *testing.T) {
	tests := []struct {
		name string
		obj  client.Object
		want OAMObjectReference
	}{
		{
			name: "base",
			obj:  &mockObject{},
			want: OAMObjectReference{},
		},
		{
			name: "base",
			obj:  &mockObject2{},
			want: OAMObjectReference{
				Component: "default",
				Trait:     "default",
				Env:       "default",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NewOAMObjectReferenceFromObject(tt.obj); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NewOAMObjectReferenceFromObject() = %v, want %v", got, tt.want)
			}
		})
	}
}

type mockObject3 struct {
	runtime.Object
}

func TestRawExtensionPointer_MarshalJSON(t *testing.T) {
	o := &mockObject3{}
	b, _ := json.Marshal(o)
	tests := []struct {
		name         string
		RawExtension *runtime.RawExtension
		want         []byte
		wantErr      bool
	}{
		{
			name:         "base",
			RawExtension: nil,
			want:         nil,
		},
		{
			name: "raw is nil, object is not nil",
			RawExtension: &runtime.RawExtension{
				Raw:    nil,
				Object: o,
			},
			want: b,
		},
		{
			name:         "raw is nil, object is nil",
			RawExtension: &runtime.RawExtension{},
			want:         []byte("null"),
		},
		{
			name: "raw is not nil",
			RawExtension: &runtime.RawExtension{
				Raw: []byte{},
			},
			want: []byte{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			re := RawExtensionPointer{
				RawExtension: tt.RawExtension,
			}
			got, err := re.MarshalJSON()
			if (err != nil) != tt.wantErr {
				t.Errorf("RawExtensionPointer.MarshalJSON() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("RawExtensionPointer.MarshalJSON() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseApplicationConditionType(t *testing.T) {
	tests := []struct {
		name    string
		s       string
		want    ApplicationConditionType
		wantErr bool
	}{
		{
			name: "base",
			s:    "Parsed",
			want: ParsedCondition,
		},
		{
			name:    "unknown condition type",
			s:       "NotFound",
			want:    -1,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseApplicationConditionType(tt.s)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseApplicationConditionType() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ParseApplicationConditionType() = %v, want %v", got, tt.want)
			}
		})
	}
}

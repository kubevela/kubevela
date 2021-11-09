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

package v1alpha2

import (
	"fmt"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/stretchr/testify/require"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

var app = Application{
	Spec: ApplicationSpec{
		Components: []ApplicationComponent{{
			Name:         "test-component",
			WorkloadType: "worker",
			Traits:       []ApplicationTrait{},
			Scopes:       map[string]string{},
		}},
	},
}

type errType struct {
}

func (*errType) Hub() {}

func (*errType) DeepCopyObject() runtime.Object {
	return nil
}

func (*errType) GetObjectKind() schema.ObjectKind {
	return nil
}

func TestApplicationV1alpha2ToV1beta1(t *testing.T) {
	r := require.New(t)
	expected := &v1beta1.Application{}
	ApplicationV1alpha2ToV1beta1(&app, expected)

	r.Equal(expected, &v1beta1.Application{
		Spec: v1beta1.ApplicationSpec{
			Components: []common.ApplicationComponent{{
				Name:       "test-component",
				Type:       "worker",
				Properties: &runtime.RawExtension{},
				Traits:     []common.ApplicationTrait{},
				Scopes:     map[string]string{},
			}},
		},
	})
}

func TestConvertTo(t *testing.T) {
	r := require.New(t)
	expected := &v1beta1.Application{}
	err := app.ConvertTo(expected)
	r.NoError(err)
	r.Equal(expected, &v1beta1.Application{
		Spec: v1beta1.ApplicationSpec{
			Components: []common.ApplicationComponent{{
				Name:       "test-component",
				Type:       "worker",
				Properties: &runtime.RawExtension{},
				Traits:     []common.ApplicationTrait{},
				Scopes:     map[string]string{},
			}},
		},
	})

	errCase := &errType{}
	err = app.ConvertTo(errCase)
	r.Equal(err, fmt.Errorf("unsupported convertTo object *v1alpha2.errType"))
}

func TestConvertFrom(t *testing.T) {
	r := require.New(t)
	to := &Application{}
	from := &v1beta1.Application{
		Spec: v1beta1.ApplicationSpec{
			Components: []common.ApplicationComponent{{
				Name:       "test-component",
				Type:       "worker",
				Properties: &runtime.RawExtension{},
				Traits:     []common.ApplicationTrait{},
				Scopes:     map[string]string{},
			}},
		},
	}
	err := to.ConvertFrom(from)
	r.NoError(err)
	r.Equal(to.Spec, app.Spec)

	errCase := &errType{}
	err = app.ConvertFrom(errCase)
	r.Equal(err, fmt.Errorf("unsupported ConvertFrom object *v1alpha2.errType"))
}

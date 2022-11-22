/*
Copyright 2022 The KubeVela Authors.

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

package cli

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

func TestFormatApplicationString(t *testing.T) {
	var (
		str string
		err error
	)

	app := &v1beta1.Application{}
	app.SetGroupVersionKind(v1beta1.ApplicationKindVersionKind)
	// This should not be preset in the formatted string
	app.ManagedFields = []v1.ManagedFieldsEntry{
		{
			Manager:     "",
			Operation:   "",
			APIVersion:  "",
			Time:        nil,
			FieldsType:  "",
			FieldsV1:    nil,
			Subresource: "",
		},
	}
	app.SetName("app-name")

	_, err = formatApplicationString("", app)
	assert.ErrorContains(t, err, "no format", "no format provided, should error out")

	_, err = formatApplicationString("invalid", app)
	assert.ErrorContains(t, err, "not supported", "invalid format provided, should error out")

	str, err = formatApplicationString("yaml", app)
	assert.NoError(t, err)
	assert.Equal(t, true, strings.Contains(str, `apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  creationTimestamp: null
  name: app-name
spec:
  components: null
status: {}
`), "formatted yaml is not correct")

	str, err = formatApplicationString("json", app)
	assert.NoError(t, err)
	assert.Equal(t, true, strings.Contains(str, `{
  "kind": "Application",
  "apiVersion": "core.oam.dev/v1beta1",
  "metadata": {
    "name": "app-name",
    "creationTimestamp": null
  },
  "spec": {
    "components": null
  },
  "status": {}
}`), "formatted json is not correct")

	_, err = formatApplicationString("jsonpath", app)
	assert.ErrorContains(t, err, "jsonpath template", "no jsonpath template provided, should not pass")

	str, err = formatApplicationString("jsonpath={.apiVersion}", app)
	assert.NoError(t, err)
	assert.Equal(t, str, "core.oam.dev/v1beta1")

	str, err = formatApplicationString("jsonpath={.spec.components[?(@.name==\"test-server\")].type}", &v1beta1.Application{
		ObjectMeta: v1.ObjectMeta{
			Name:      "test-app",
			Namespace: "dev",
		},
		Spec: v1beta1.ApplicationSpec{
			Components: []common.ApplicationComponent{
				{
					Name: "test-server",
					Type: "webservice",
				},
			},
		},
	})
	assert.NoError(t, err)
	assert.Equal(t, str, "webservice")
}

func TestConvertApplicationRevisionTo(t *testing.T) {

	type Exp struct {
		out string
		err string
	}

	cases := map[string]struct {
		format string
		apprev *v1beta1.ApplicationRevision
		exp    Exp
	}{
		"no format":         {format: "", apprev: &v1beta1.ApplicationRevision{}, exp: Exp{out: "", err: "no format provided"}},
		"no support format": {format: "jsonnet", apprev: &v1beta1.ApplicationRevision{}, exp: Exp{out: "", err: "jsonnet is not supported"}},
		"yaml": {format: "yaml", apprev: &v1beta1.ApplicationRevision{
			TypeMeta: v1.TypeMeta{
				Kind:       "ApplicationRevision",
				APIVersion: "core.oam.dev/v1beta1",
			},
			ObjectMeta: v1.ObjectMeta{
				Name:      "test-apprev",
				Namespace: "dev",
			},
			Spec: v1beta1.ApplicationRevisionSpec{
				ApplicationRevisionCompressibleFields: v1beta1.ApplicationRevisionCompressibleFields{
					Application: v1beta1.Application{
						ObjectMeta: v1.ObjectMeta{
							Name:      "test-app",
							Namespace: "dev",
						},
					},
				},
			},
		}, exp: Exp{out: `apiVersion: core.oam.dev/v1beta1
kind: ApplicationRevision
metadata:
  creationTimestamp: null
  name: test-apprev
  namespace: dev
spec:
  application:
    metadata:
      creationTimestamp: null
      name: test-app
      namespace: dev
    spec:
      components: null`, err: ""}},
		"json": {format: "json", apprev: &v1beta1.ApplicationRevision{
			TypeMeta: v1.TypeMeta{
				Kind:       "ApplicationRevision",
				APIVersion: "core.oam.dev/v1beta1",
			},
			ObjectMeta: v1.ObjectMeta{
				Name:      "test-apprev",
				Namespace: "dev",
			},
			Spec: v1beta1.ApplicationRevisionSpec{
				ApplicationRevisionCompressibleFields: v1beta1.ApplicationRevisionCompressibleFields{
					Application: v1beta1.Application{
						ObjectMeta: v1.ObjectMeta{
							Name:      "test-app",
							Namespace: "dev",
						},
					},
				},
			},
		}, exp: Exp{out: `{
  "kind": "ApplicationRevision",
  "apiVersion": "core.oam.dev/v1beta1",
  "metadata": {
    "name": "test-apprev",
    "namespace": "dev",
    "creationTimestamp": null
  },
  "spec": {
    "application": {
      "metadata": {
        "name": "test-app",
        "namespace": "dev",
        "creationTimestamp": null
      },
      "spec": {
        "components": null`, err: ""}},
		"jsonpath": {format: "jsonpath={.apiVersion}", apprev: &v1beta1.ApplicationRevision{
			TypeMeta: v1.TypeMeta{
				Kind:       "ApplicationRevision",
				APIVersion: "core.oam.dev/v1beta1",
			},
			ObjectMeta: v1.ObjectMeta{
				Name:      "test-apprev",
				Namespace: "dev",
			},
			Spec: v1beta1.ApplicationRevisionSpec{
				ApplicationRevisionCompressibleFields: v1beta1.ApplicationRevisionCompressibleFields{
					Application: v1beta1.Application{
						ObjectMeta: v1.ObjectMeta{
							Name:      "test-app",
							Namespace: "dev",
						},
					},
				},
			},
		}, exp: Exp{out: "core.oam.dev/v1beta1", err: ""}},
		"jsonpath filter expression": {format: "jsonpath={.spec.application.spec.components[?(@.name==\"test-server\")].type}", apprev: &v1beta1.ApplicationRevision{
			TypeMeta: v1.TypeMeta{
				Kind:       "ApplicationRevision",
				APIVersion: "core.oam.dev/v1beta1",
			},
			ObjectMeta: v1.ObjectMeta{
				Name:      "test-apprev",
				Namespace: "dev",
			},
			Spec: v1beta1.ApplicationRevisionSpec{
				ApplicationRevisionCompressibleFields: v1beta1.ApplicationRevisionCompressibleFields{
					Application: v1beta1.Application{
						ObjectMeta: v1.ObjectMeta{
							Name:      "test-app",
							Namespace: "dev",
						},
						Spec: v1beta1.ApplicationSpec{
							Components: []common.ApplicationComponent{
								{
									Name: "test-server",
									Type: "webservice",
								},
							},
						},
					},
				},
			},
		}, exp: Exp{out: "webservice", err: ""}},
		"jsonpath with error": {format: "jsonpath", apprev: &v1beta1.ApplicationRevision{
			TypeMeta: v1.TypeMeta{
				Kind:       "ApplicationRevision",
				APIVersion: "core.oam.dev/v1beta1",
			},
			ObjectMeta: v1.ObjectMeta{
				Name:      "test-apprev",
				Namespace: "dev1",
			},
			Spec: v1beta1.ApplicationRevisionSpec{
				ApplicationRevisionCompressibleFields: v1beta1.ApplicationRevisionCompressibleFields{
					Application: v1beta1.Application{
						ObjectMeta: v1.ObjectMeta{
							Name:      "test-app",
							Namespace: "dev1",
						},
					},
				},
			},
		}, exp: Exp{out: "", err: "jsonpath template format specified but no template given"}},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			out, err := convertApplicationRevisionTo(tc.format, tc.apprev)
			if err != nil {
				assert.Equal(t, tc.exp.err, err.Error())
			}
			assert.Contains(t, out, tc.exp.out)
		})
	}
}

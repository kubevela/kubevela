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

package services

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/oam-dev/kubevela/pkg/apiserver/rest/apis"
	"github.com/oam-dev/kubevela/pkg/utils/common"
)

func TestDefinitionCreate(t *testing.T) {
	cw := fake.NewClientBuilder().WithScheme(common.Scheme).Build()
	defSvc := NewDefinitionService(cw)

	tests := map[string]struct {
		defReq      *apis.DefinitionRequest
		rawReq      []byte
		name        string
		namespace   string
		format      string
		expHttpCode int
		expErr      string
	}{
		"normal create definition with json": {
			defReq: &apis.DefinitionRequest{
				APIVersion: "core.oam.dev/v1beta1",
				Kind:       "TraitDefinition",
				Spec:       defSpecTemplate,
			},
			format:      "json",
			expHttpCode: 200,
			name:        "testDef",
			namespace:   "testNamespace",
		},
		"normal create definition with cue": {
			defReq: &apis.DefinitionRequest{
				CUEString: CUEStringTemplate,
			},
			format:      "cue",
			expHttpCode: 200,
			name:        "testDef",
			namespace:   "testNamespace",
		},
	}

	for casename, c := range tests {
		var err error
		if c.defReq != nil {
			c.rawReq, err = json.Marshal(c.defReq)
			assert.NoError(t, err, casename)
		}
		req := httptest.NewRequest(http.MethodPost, "/definitions/:defname?namespace="+c.namespace+"&format="+c.format, bytes.NewBuffer(c.rawReq))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		echoCtx := echo.New().NewContext(req, rec)
		echoCtx.SetParamNames("namespace", "defname", "format")
		echoCtx.SetParamValues(c.namespace, c.name, c.format)

		err = defSvc.CreateDefinition(echoCtx)
		assert.NoError(t, err, casename)

		// check response
		assert.Equal(t, c.expHttpCode, rec.Code, casename)
		if c.expErr != "" { // compare return error with map type
			gotResp := map[string]string{}
			err = json.Unmarshal(rec.Body.Bytes(), &gotResp)
			assert.NoError(t, err, casename)
			assert.True(t, strings.Contains(gotResp["error"], c.expErr), casename)
		} else { // check definition spec in fake cluster

			objs := unstructured.Unstructured{}
			objs.SetGroupVersionKind(schema.GroupVersionKind{
				Group:   v1beta1.Group,
				Version: v1beta1.Version,
				Kind:    c.defReq.Kind,
			})
			err := cw.Get(context.TODO(),client.ObjectKey{Namespace: c.namespace,Name: c.name}, &objs)
			assert.NoError(t, err, casename)

			//var def common2.Definition
			//def.SetGVK(c.defReq.Kind)
			//err = cw.Get(context.TODO(), client.ObjectKey{Namespace: c.namespace, Name: c.name}, &def)
			//assert.NoError(t, err, casename)
		}
	}
}

var CUEStringTemplate = `
annotations: {
	type: "trait"
	annotations: {}
	labels: {}
	description: "Add annotations for your Workload."
	attributes: {
		podDisruptive: true
		appliesToWorkloads: ["deployments.apps"]
	}
}
template: {
	patch: spec: template: metadata: annotations: {
		for k, v in parameter {
			"\(k)": v
		}
	}
	parameter: [string]: string
}`

var defSpecTemplate = `
schematic:
    cue:
      template: |
        patch: spec: template: metadata: labels: {
        	for k, v in parameter {
        		"\(k)": v
        	}
        }
        parameter: [string]: string`

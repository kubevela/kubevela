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
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	common2 "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/apis"
	"github.com/oam-dev/kubevela/pkg/utils/common"
)

func TestApplicationCreateOrUpdate(t *testing.T) {
	cw := fake.NewClientBuilder().WithScheme(common.Scheme).Build()
	appSvc := NewApplicationService(cw)

	appComp1 := common2.ApplicationComponent{
		Name:       "mycomp",
		Type:       "webservice",
		Properties: runtime.RawExtension{Raw: []byte(`{"image":"nginx:v1"}`)},
	}
	appComp2 := common2.ApplicationComponent{
		Name:       "mycomp2",
		Type:       "webservice",
		Properties: runtime.RawExtension{Raw: []byte(`{"image":"nginx:v2"}`)},
	}
	tests := map[string]struct {
		appReq      *apis.ApplicationRequest
		rawReq      []byte
		name        string
		namespace   string
		expHttpCode int
		expErr      string
		expApp      *v1beta1.Application
	}{
		"normal create with only component": {
			appReq: &apis.ApplicationRequest{
				Components: []common2.ApplicationComponent{appComp1},
			},
			expHttpCode: 200,
			name:        "myapp",
			namespace:   "mynamespace",
			expApp: &v1beta1.Application{
				Spec: v1beta1.ApplicationSpec{
					Components: []common2.ApplicationComponent{appComp1},
				},
			},
		},
		"create with bind error": {
			rawReq:      []byte("XXXX"),
			expHttpCode: 400,
			name:        "myapp",
			namespace:   "mynamespace",
			expErr:      "invalid request body: code=400",
		},
		"normal update with component and trait": {
			appReq: &apis.ApplicationRequest{
				Components: []common2.ApplicationComponent{appComp1, appComp2},
			},
			expHttpCode: 200,
			name:        "myapp",
			namespace:   "mynamespace",
			expApp: &v1beta1.Application{
				Spec: v1beta1.ApplicationSpec{
					Components: []common2.ApplicationComponent{appComp1, appComp2},
				},
			},
		},
	}
	for casename, c := range tests {
		var err error
		if c.appReq != nil {
			c.rawReq, err = json.Marshal(c.appReq)
			assert.NoError(t, err, casename)
		}
		req := httptest.NewRequest(http.MethodPost, "/", bytes.NewBuffer(c.rawReq))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		echoCtx := echo.New().NewContext(req, rec)
		echoCtx.SetParamNames("namespace", "appname")
		echoCtx.SetParamValues(c.namespace, c.name)

		err = appSvc.CreateOrUpdateApplication(echoCtx)
		assert.NoError(t, err, casename)

		// check response
		assert.Equal(t, c.expHttpCode, rec.Code, casename)
		gotResp := map[string]string{}
		err = json.Unmarshal(rec.Body.Bytes(), &gotResp)
		assert.NoError(t, err, casename)
		if c.expErr != "" {
			assert.True(t, strings.Contains(gotResp["error"], c.expErr), casename)
		}

		if len(c.expErr) > 0 {
			continue
		}
		var appObj v1beta1.Application
		err = cw.Get(context.TODO(), client.ObjectKey{Namespace: c.namespace, Name: c.name}, &appObj)
		assert.NoError(t, err, casename)
		assert.Equal(t, c.expApp.Spec, appObj.Spec, casename)
	}
}

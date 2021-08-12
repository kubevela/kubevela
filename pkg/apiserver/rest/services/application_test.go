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
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	common2 "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/apis"
	"github.com/oam-dev/kubevela/pkg/utils/common"
)

func TestApplicationCreateOrUpdate(t *testing.T) {
	cw := fake.NewClientBuilder().WithScheme(common.Scheme).Build()
	appSvc := NewApplicationService(cw)

	tests := map[string]struct {
		appReq      *apis.ApplicationRequest
		expHttpCode int
		expErr      map[string]string
	}{
		"normal create with only component": {
			appReq: &apis.ApplicationRequest{
				Components: []common2.ApplicationComponent{
					{
						Name:       "mycomp",
						Type:       "webservice",
						Properties: runtime.RawExtension{Raw: []byte(`{"image":"nginx:v1"}`)},
					},
				},
			},
			expHttpCode: 200,
			expErr:      map[string]string{},
		},
	}
	for cname, c := range tests {
		b, err := json.Marshal(c.appReq)
		assert.NoError(t, err, cname)
		req := httptest.NewRequest(http.MethodPost, "/", bytes.NewBuffer(b))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		echoCtx := echo.New().NewContext(req, rec)
		echoCtx.SetParamNames("namespace", "appname")
		echoCtx.SetParamValues("mynamespace", "myapp")

		err = appSvc.CreateOrUpdateApplication(echoCtx)
		assert.NoError(t, err, cname)

		// check response
		assert.Equal(t, c.expHttpCode, rec.Code, cname)
		gotResp := map[string]string{}
		err = json.Unmarshal(rec.Body.Bytes(), &gotResp)
		assert.NoError(t, err, cname)
		assert.Equal(t, c.expErr, gotResp)
	}
}

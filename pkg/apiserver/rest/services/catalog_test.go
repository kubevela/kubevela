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
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/apis"
	"github.com/oam-dev/kubevela/pkg/utils/common"
)

func TestCatalogGet(t *testing.T) {
	cw := fake.NewClientBuilder().WithScheme(common.Scheme).Build()
	catalogService := NewCatalogService(cw)

	tests := map[string]struct {
		rawReq      []byte
		name        string
		namespace   string
		expHttpCode int
		expErr      string
		expApp      *v1beta1.Application
	}{
		"normal get test for catalog": {
			expHttpCode: 200,
			name:        "testName",
			namespace:   "testNamespace",
		},
	}

	// create an catalog for get
	cr := &apis.CatalogRequest{
		Name: "test",
	}
	rawReq, err := json.Marshal(cr)
	assert.NoError(t, err, "marshal request for create catalog. ")
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewBuffer(rawReq))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	echoCtx := echo.New().NewContext(req, rec)

	err = catalogService.AddCatalog(echoCtx)
	assert.NoError(t, err, "create catalog for get test")
	checkCatalogResponse(t, rec, cr, http.StatusCreated)

	// get and check for catalog details
	for caseName, c := range tests {
		req = httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec = httptest.NewRecorder()
		echoCtx = echo.New().NewContext(req, rec)
		echoCtx.SetPath("/v1/catalogs/:catalogName")
		echoCtx.SetParamNames("catalogName")
		echoCtx.SetParamValues(cr.Name)

		err = catalogService.GetCatalog(echoCtx)
		assert.NoError(t, err, caseName)
		checkCatalogResponse(t, rec, cr, c.expHttpCode)
	}
}

func checkCatalogResponse(t *testing.T, rec *httptest.ResponseRecorder, cr *apis.CatalogRequest, httpcode int) {
	assert.Equal(t, rec.Code, httpcode)

	get := &apis.CatalogResponse{}
	err := json.Unmarshal(rec.Body.Bytes(), get)
	assert.NoError(t, err, "unmarshal rec body")

	assert.Equal(t, get.Catalog.Name, cr.Name)
	assert.NotEqualValues(t, get.Catalog.UpdatedAt, 0)
}

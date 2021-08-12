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

	"github.com/labstack/echo/v4"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/oam-dev/kubevela/pkg/apiserver/rest/apis"
)

var _ = Describe("Test Catalog Service", func() {

	var catalogService *CatalogService

	BeforeEach(func() {
		catalogService = NewCatalogService(k8sClient)
	})

	AfterEach(func() {
	})

	It("should add catalog successfully", func() {
		e := echo.New()
		cr := &apis.CatalogRequest{
			Name: "test",
		}
		b, err := json.Marshal(cr)
		Expect(err).To(BeNil())
		req := httptest.NewRequest(http.MethodPost, "/", bytes.NewBuffer(b))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		Expect(catalogService.AddCatalog(c)).To(BeNil())
		checkCatalogResponse(rec, cr, http.StatusCreated)

		req = httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec = httptest.NewRecorder()
		c = e.NewContext(req, rec)
		c.SetPath("/dashboard/catalogs/:catalogName")
		c.SetParamNames("catalogName")
		c.SetParamValues(cr.Name)

		Expect(catalogService.GetCatalog(c)).To(BeNil())
		checkCatalogResponse(rec, cr, http.StatusOK)
	})
})

func checkCatalogResponse(rec *httptest.ResponseRecorder, cr *apis.CatalogRequest, httpcode int) {
	Expect(rec.Code).To(Equal(httpcode))

	get := &apis.CatalogResponse{}
	err := json.Unmarshal(rec.Body.Bytes(), get)
	Expect(err).To(BeNil())

	Expect(get.Catalog.Name).To(Equal(cr.Name))
	Expect(get.Catalog.UpdatedAt).NotTo(BeZero())
}

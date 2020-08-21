package integration_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("example test", func() {

	It("Test get environment", func() {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/api/envs/test", nil)
		restServer.ServeHTTP(w, req)

		Expect(w.Code).Should(Equal(http.StatusOK))
		// TODO: unmarshall the body and check
		fmt.Println(w.Body.String())
	})
})

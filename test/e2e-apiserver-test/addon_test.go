package e2e_apiserver

import (
	"bytes"
	"encoding/json"
	"net/http"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/oam-dev/kubevela/pkg/apiserver/model"
	apis "github.com/oam-dev/kubevela/pkg/apiserver/rest/apis/v1"
)

const baseURL = "http://127.0.0.1:8000"

func post(path string, body interface{}) *http.Response {
	b, err := json.Marshal(body)
	Expect(err).Should(BeNil())

	res, err := http.Post(baseURL+path, "application/json", bytes.NewBuffer(b))
	Expect(err).Should(BeNil())
	return res
}

var _ = Describe("Test addon rest api", func() {
	It("should create and delete an addon registry", func() {
		defer GinkgoRecover()
		req := apis.CreateAddonRegistryRequest{
			Name: "test-addon-registry-1",
			Git: &model.GitAddonSource{
				URL: "test_url",
				Dir: "test_dir",
			},
		}
		res := post("/api/v1/addon_registries", req)
		Expect(res).ShouldNot(BeNil())
		Expect(res.StatusCode).Should(Equal(200))
		Expect(res.Body).ShouldNot(BeNil())

		defer res.Body.Close()

		var rmeta apis.AddonRegistryMeta
		err := json.NewDecoder(res.Body).Decode(&rmeta)

		Expect(err).Should(BeNil())
		Expect(rmeta.Name).Should(Equal(req.Name))
		Expect(rmeta.Git).Should(Equal(req.Git))
	})

	It("should list all addons", func() {})

	It("should enable and disable an addon", func() {
		defer GinkgoRecover()
		req := apis.EnableAddonRequest{
			Args: map[string]string{},
		}
		res := post("/api/v1/addons/enable?name=fluxcd", req)
		Expect(res).ShouldNot(BeNil())
		Expect(res.StatusCode).Should(Equal(200))
		Expect(res.Body).ShouldNot(BeNil())

		defer res.Body.Close()

		var statusRes apis.AddonStatusResponse
		err := json.NewDecoder(res.Body).Decode(&statusRes)

		Expect(err).Should(BeNil())
		Expect(statusRes.Phase).Should(Equal(apis.AddonPhaseEnabling))

		// Wait for addon enabled


	})
})

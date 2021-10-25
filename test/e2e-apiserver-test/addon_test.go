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

var _ = Describe("Test addon rest api", func() {
	It("should create and delete an addon registry", func() {
		defer GinkgoRecover()
		var req = apis.CreateAddonRegistryRequest{
			Name: "test-addon-registry-1",
			Git: &model.GitAddonSource{
				URL: "test_url",
				Dir: "test_dir",
			},
		}

		bodyByte, err := json.Marshal(req)
		Expect(err).Should(BeNil())

		res, err := http.Post("http://127.0.0.1:8000/api/v1/addon_registries", "application/json", bytes.NewBuffer(bodyByte))
		Expect(err).Should(BeNil())

		Expect(res).ShouldNot(BeNil())
		Expect(res.StatusCode).Should(Equal(200))
		Expect(res.Body).ShouldNot(BeNil())

		defer res.Body.Close()

		var rmeta apis.AddonRegistryMeta
		err = json.NewDecoder(res.Body).Decode(&rmeta)

		Expect(err).Should(BeNil())
		Expect(rmeta.Name).Should(Equal(req.Name))
		Expect(rmeta.Git).Should(Equal(req.Git))
	})

	It("should list all addons", func() {})

	It("should enable and disable an addon", func() {})
})

package e2e_apiserver

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"

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

func get(path string) *http.Response {
	res, err := http.Get(baseURL + path)
	Expect(err).Should(BeNil())
	return res
}

var _ = Describe("Test addon rest api", func() {
	It("should add a registry and list addons from it and delete the registry", func() {
		defer GinkgoRecover()

		By("add registry")
		createReq := apis.CreateAddonRegistryRequest{
			Name: "test-addon-registry-1",
			Git: &model.GitAddonSource{
				URL:   "https://github.com/oam-dev/catalog",
				Path:  "addon/",
				Token: os.Getenv("GITHUB_TOKEN"),
			},
		}
		createRes := post("/api/v1/addon_registries", createReq)
		Expect(createRes).ShouldNot(BeNil())
		Expect(createRes.StatusCode).Should(Equal(200))
		Expect(createRes.Body).ShouldNot(BeNil())

		defer createRes.Body.Close()

		var rmeta apis.AddonRegistryMeta
		err := json.NewDecoder(createRes.Body).Decode(&rmeta)
		Expect(err).Should(BeNil())
		Expect(rmeta.Name).Should(Equal(createReq.Name))
		Expect(rmeta.Git).Should(Equal(createReq.Git))

		By("list addons")
		listRes := get("/api/v1/addons/")
		defer listRes.Body.Close()

		var lres apis.ListAddonResponse
		err = json.NewDecoder(listRes.Body).Decode(&lres)
		Expect(err).Should(BeNil())
		Expect(lres.Addons).ShouldNot(BeZero())
		firstAddon := lres.Addons[0]
		Expect(firstAddon.Name).Should(Equal("fluxcd"))

		By("delete registry")
		deleteReq, err := http.NewRequest(http.MethodDelete, baseURL+"/api/v1/addon_registries/"+createReq.Name, nil)
		Expect(err).Should(BeNil())
		deleteRes, err := http.DefaultClient.Do(deleteReq)
		Expect(err).Should(BeNil())
		Expect(deleteRes).ShouldNot(BeNil())
		Expect(deleteRes.StatusCode).Should(Equal(200))
	})

	It("should enable and disable an addon", func() {
		defer GinkgoRecover()
		req := apis.EnableAddonRequest{
			Args: map[string]string{},
		}
		testAddon := "fluxcd"
		res := post("/api/v1/addons/enable?name="+testAddon, req)
		Expect(res).ShouldNot(BeNil())
		Expect(res.StatusCode).Should(Equal(200))
		Expect(res.Body).ShouldNot(BeNil())

		defer res.Body.Close()

		var statusRes apis.AddonStatusResponse
		err := json.NewDecoder(res.Body).Decode(&statusRes)

		Expect(err).Should(BeNil())
		Expect(statusRes.Phase).Should(Equal(apis.AddonPhaseEnabling))

		// Wait for addon enabled

		period := 20 * time.Second
		timeout := 5 * time.Minute
		err = wait.PollImmediate(period, timeout, func() (done bool, err error) {
			res = get("/api/v1/addons/status?name=" + testAddon)
			err = json.NewDecoder(res.Body).Decode(&statusRes)
			Expect(err).Should(BeNil())
			if statusRes.Phase == apis.AddonPhaseEnabled {
				return true, nil
			}
			return false, nil
		})
		Expect(err).Should(BeNil())

		res = post("/api/v1/addons/disable?name="+testAddon, req)
		Expect(res).ShouldNot(BeNil())
		Expect(res.StatusCode).Should(Equal(200))
		Expect(res.Body).ShouldNot(BeNil())

		err = json.NewDecoder(res.Body).Decode(&statusRes)
		Expect(err).Should(BeNil())

	})
})

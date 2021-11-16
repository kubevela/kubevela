package e2e_apiserver

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/oam-dev/kubevela/pkg/addon"
	apis "github.com/oam-dev/kubevela/pkg/apiserver/rest/apis/v1"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/pkg/utils/common"
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
	createReq := apis.CreateAddonRegistryRequest{
		Name: "test-addon-registry-1",
		Git: &addon.GitAddonSource{
			URL:   "https://github.com/oam-dev/catalog",
			Path:  "addons/",
			Token: os.Getenv("GITHUB_TOKEN"),
		},
	}
	It("should add a registry and list addons from it", func() {
		defer GinkgoRecover()

		By("add registry")
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
		Expect(firstAddon.Name).Should(Equal("example"))

	})

	It("should enable and disable an addon", func() {
		// todo(qiaozp) we should remove this namespace creation. This should be solved with a application template.
		ns := v1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "flux-system",
			},
		}
		args := common.Args{}
		k8sClient, err := args.GetClient()
		Expect(err).Should(BeNil())
		Expect(k8sClient.Create(context.Background(), &ns)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		defer GinkgoRecover()
		req := apis.EnableAddonRequest{
			Args: map[string]string{
				"example": "test-args",
			},
		}
		testAddon := "example"
		res := post("/api/v1/addons/"+testAddon+"/enable", req)
		Expect(res).ShouldNot(BeNil())
		Expect(res.StatusCode).Should(Equal(200))
		Expect(res.Body).ShouldNot(BeNil())

		defer res.Body.Close()

		var statusRes apis.AddonStatusResponse
		err = json.NewDecoder(res.Body).Decode(&statusRes)

		Expect(err).Should(BeNil())
		Expect(statusRes.Phase).Should(Equal(apis.AddonPhaseEnabling))

		// Wait for addon enabled

		period := 20 * time.Second
		timeout := 5 * time.Minute
		err = wait.PollImmediate(period, timeout, func() (done bool, err error) {
			res = get("/api/v1/addons/" + testAddon + "/status")
			err = json.NewDecoder(res.Body).Decode(&statusRes)
			Expect(err).Should(BeNil())
			if statusRes.Phase == apis.AddonPhaseEnabled {
				return true, nil
			}
			return false, nil
		})
		Expect(err).Should(BeNil())

		res = post("/api/v1/addons/"+testAddon+"/disable", req)
		Expect(res).ShouldNot(BeNil())
		Expect(res.StatusCode).Should(Equal(200))
		Expect(res.Body).ShouldNot(BeNil())

		err = json.NewDecoder(res.Body).Decode(&statusRes)
		Expect(err).Should(BeNil())
	})

	It("should delete test registry", func() {
		defer GinkgoRecover()
		deleteReq, err := http.NewRequest(http.MethodDelete, baseURL+"/api/v1/addon_registries/"+createReq.Name, nil)
		Expect(err).Should(BeNil())
		deleteRes, err := http.DefaultClient.Do(deleteReq)
		Expect(err).Should(BeNil())
		Expect(deleteRes).ShouldNot(BeNil())
		Expect(deleteRes.StatusCode).Should(Equal(200))
	})
})

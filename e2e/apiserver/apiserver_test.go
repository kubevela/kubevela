package e2e

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/cloud-native-application/rudrx/e2e"

	"github.com/cloud-native-application/rudrx/api/types"

	"github.com/cloud-native-application/rudrx/pkg/server/util"

	"github.com/cloud-native-application/rudrx/pkg/server/apis"

	"github.com/onsi/gomega"

	"github.com/onsi/ginkgo"
)

var envHelloMeta = types.EnvMeta{
	Name:      "env-e2e-hello",
	Namespace: "env-e2e-hello",
}

var envWorldMeta = types.EnvMeta{
	Name:      "env-e2e-world",
	Namespace: "env-e2e-world",
}

var _ = ginkgo.Describe("API Env", func() {
	e2e.APIEnvInitContext("post /envs/", envHelloMeta)

	ginkgo.Context("get /envs/:envName", func() {
		ginkgo.It("should get an env", func() {
			resp, err := http.Get(util.URL("/envs/" + envHelloMeta.Name))
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			defer resp.Body.Close()
			result, err := ioutil.ReadAll(resp.Body)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			var r apis.Response
			err = json.Unmarshal(result, &r)
			gomega.Expect(http.StatusOK).To(gomega.Equal(r.Code))
			//TODO(zzxwill) Need to compare r.Data with envMeta
		})
	})

	e2e.APIEnvInitContext("post /envs/", envWorldMeta)

	ginkgo.Context("switch /envs/:envName", func() {
		ginkgo.It("should switch an env", func() {
			req, err := http.NewRequest("PATCH", util.URL("/envs/"+envHelloMeta.Name), nil)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			resp, err := http.DefaultClient.Do(req)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			defer resp.Body.Close()
			result, err := ioutil.ReadAll(resp.Body)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			var r apis.Response
			err = json.Unmarshal(result, &r)
			gomega.Expect(http.StatusOK).To(gomega.Equal(r.Code))
			content := fmt.Sprintf("Switch env succeed, current env is " + envHelloMeta.Name)
			gomega.Expect(r.Data.(string)).To(gomega.ContainSubstring(content))
		})
	})

	ginkgo.Context("get /envs/", func() {
		ginkgo.It("should get an env", func() {
			resp, err := http.Get(util.URL("/envs/"))
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			defer resp.Body.Close()
			result, err := ioutil.ReadAll(resp.Body)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			var r apis.Response
			err = json.Unmarshal(result, &r)
			gomega.Expect(http.StatusOK).To(gomega.Equal(r.Code))
			//TODO(zzxwill) Need to compare r.Data with envMeta
		})
	})

	ginkgo.Context("delete /envs/:envName", func() {
		ginkgo.It("should delete an env", func() {
			req, err := http.NewRequest("DELETE", util.URL("/envs/"+envWorldMeta.Name), nil)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			resp, err := http.DefaultClient.Do(req)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			defer resp.Body.Close()
			result, err := ioutil.ReadAll(resp.Body)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			var r apis.Response
			err = json.Unmarshal(result, &r)
			gomega.Expect(http.StatusOK).To(gomega.Equal(r.Code))
			gomega.Expect(r.Data.(string)).To(gomega.ContainSubstring(envWorldMeta.Name + " deleted"))
		})
	})
})

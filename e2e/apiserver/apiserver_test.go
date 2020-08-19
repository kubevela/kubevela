package e2e

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/cloud-native-application/rudrx/e2e"

	"github.com/cloud-native-application/rudrx/api/types"

	"github.com/cloud-native-application/rudrx/pkg/server/util"

	"github.com/cloud-native-application/rudrx/pkg/server/apis"

	"github.com/onsi/gomega"

	"github.com/onsi/ginkgo"
)

var (
	envHelloMeta = types.EnvMeta{
		Name:      "env-e2e-hello",
		Namespace: "env-e2e-hello",
	}

	envWorldMeta = types.EnvMeta{
		Name:      "env-e2e-world",
		Namespace: "env-e2e-world",
	}

	workloadType = "containerized"
	workloadName = "app-e2e-api-hello"

	workloadRunBodyWithoutImageFlag = apis.WorkloadRunBody{
		EnvName:      envHelloMeta.Name,
		WorkloadName: workloadName,
		WorkloadType: workloadType,
		Flags:        []apis.WorkloadFlag{{Name: "port", Value: "80"}},
	}
	workloadRunBody = apis.WorkloadRunBody{
		EnvName:      envHelloMeta.Name,
		WorkloadName: workloadName,
		WorkloadType: workloadType,
		Flags:        []apis.WorkloadFlag{{Name: "image", Value: "nginx:1.9.4"}, {Name: "port", Value: "80"}},
	}
)

var notExistedEnvMeta = types.EnvMeta{
	Name:      "env-e2e-api-NOT-EXISTED-JUST-FOR-TEST",
	Namespace: "env-e2e-api-NOT-EXISTED-JUST-FOR-TEST",
}

var _ = ginkgo.Describe("API Env", func() {
	//API Env
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

	// API Application
	ginkgo.Context("get /envs/:envName/apps/", func() {
		ginkgo.It("should report error for not existed env", func() {
			envName := notExistedEnvMeta.Name
			url := fmt.Sprintf("/envs/%s/apps/", envName)
			resp, err := http.Get(util.URL(url))
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			defer resp.Body.Close()
			result, err := ioutil.ReadAll(resp.Body)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			var r apis.Response
			err = json.Unmarshal(result, &r)
			gomega.Expect(http.StatusInternalServerError).To(gomega.Equal(r.Code))
			expectedContent := fmt.Sprintf("env %s not exist", envName)
			gomega.Expect(r.Data.(string)).To(gomega.ContainSubstring(expectedContent))
		})
	})
})

var _ = ginkgo.Describe("API Workload", func() {

	ginkgo.Context("Post /workloads/", func() {
		ginkgo.It("run workload", func() {
			data, err := json.Marshal(&workloadRunBody)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			resp, err := http.Post(util.URL("/workloads/"), "application/json", strings.NewReader(string(data)))
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			defer resp.Body.Close()
			result, err := ioutil.ReadAll(resp.Body)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			var r apis.Response
			err = json.Unmarshal(result, &r)
			gomega.Expect(http.StatusOK).Should(gomega.Equal(r.Code))
			output := fmt.Sprintf("Creating App %s\nSUCCEED", workloadName)
			gomega.Expect(r.Data.(string)).To(gomega.ContainSubstring(output))
		})

		ginkgo.It("run workload without compulsory flag", func() {
			data, err := json.Marshal(&workloadRunBodyWithoutImageFlag)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			resp, err := http.Post(util.URL("/workloads/"), "application/json", strings.NewReader(string(data)))
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			defer resp.Body.Close()
			result, err := ioutil.ReadAll(resp.Body)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			var r apis.Response
			err = json.Unmarshal(result, &r)
			gomega.Expect(http.StatusInternalServerError).Should(gomega.Equal(r.Code))
			output := fmt.Sprintf("required flag(s) \"image\" not set")
			gomega.Expect(r.Data.(string)).To(gomega.ContainSubstring(output))
		})
	})
})

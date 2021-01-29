package e2e

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"

	"github.com/oam-dev/kubevela/pkg/server/apis"
	"github.com/oam-dev/kubevela/pkg/server/util"
)

var (
	// SystemInitContext used for test install
	SystemInitContext = func(context string) bool {
		return ginkgo.Context(context, func() {
			ginkgo.It("Install OAM runtime and vela builtin capabilities.", func() {
				output, err := LongTimeExec("vela install --wait", 180*time.Second)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				gomega.Expect(output).To(gomega.ContainSubstring("- Installing OAM Kubernetes Runtime"))
				gomega.Expect(output).To(gomega.ContainSubstring("- Installing builtin capabilities"))
				gomega.Expect(output).To(gomega.ContainSubstring("Successful applied"))
				gomega.Expect(output).To(gomega.ContainSubstring("Waiting KubeVela runtime ready to serve"))
			})
		})
	}

	// EnvInitContext used for test Env
	EnvInitContext = func(context string, envName string) bool {
		return ginkgo.Context(context, func() {
			ginkgo.It("should print environment initiation successful message", func() {
				cli := fmt.Sprintf("vela env init %s", envName)
				output, err := Exec(cli)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				expectedOutput := fmt.Sprintf("environment %s created,", envName)
				gomega.Expect(output).To(gomega.ContainSubstring(expectedOutput))
			})
		})
	}

	JsonAppFileContext = func(context, jsonAppFile string) bool {
		return ginkgo.Context(context, func() {
			ginkgo.It("Start the application through the app file in JSON format.", func() {
				writeStatus := ioutil.WriteFile("vela.json", []byte(jsonAppFile), 0644)
				gomega.Expect(writeStatus).NotTo(gomega.HaveOccurred())
				output, err := Exec("vela up -f vela.json")
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				gomega.Expect(output).NotTo(gomega.ContainSubstring("Error:"))
			})
		})
	}

	DeleteEnvFunc = func(context string, envName string) bool {
		return ginkgo.Context(context, func() {
			ginkgo.It("should print env does not exist message", func() {
				cli := fmt.Sprintf("vela env delete %s", envName)
				_, err := Exec(cli)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
			})
		})
	}

	EnvShowContext = func(context string, envName string) bool {
		return ginkgo.Context(context, func() {
			ginkgo.It("should show detailed environment message", func() {
				cli := fmt.Sprintf("vela env ls %s", envName)
				output, err := Exec(cli)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				gomega.Expect(output).To(gomega.ContainSubstring("NAME"))
				gomega.Expect(output).To(gomega.ContainSubstring("NAMESPACE"))
				gomega.Expect(output).To(gomega.ContainSubstring(envName))
			})
		})
	}

	EnvSetContext = func(context string, envName string) bool {
		return ginkgo.Context(context, func() {
			ginkgo.It("should show environment set message", func() {
				cli := fmt.Sprintf("vela env sw %s", envName)
				output, err := Exec(cli)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				expectedOutput := fmt.Sprintf("Set environment succeed, current environment is %s", envName)
				gomega.Expect(output).To(gomega.ContainSubstring(expectedOutput))
			})
		})
	}

	EnvDeleteContext = func(context string, envName string) bool {
		return ginkgo.Context(context, func() {
			ginkgo.It("should delete an environment", func() {
				cli := fmt.Sprintf("vela env delete %s", envName)
				output, err := Exec(cli)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				expectedOutput := fmt.Sprintf("%s deleted", envName)
				gomega.Expect(output).To(gomega.ContainSubstring(expectedOutput))
			})
		})
	}
	EnvDeleteCurrentUsingContext = func(context string, envName string) bool {
		return ginkgo.Context(context, func() {
			ginkgo.It("should delete all envs", func() {
				cli := fmt.Sprintf("vela env delete %s", envName)
				output, err := Exec(cli)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				expectedOutput := fmt.Sprintf("Error: you can't delete current using environment %s", envName)
				gomega.Expect(output).To(gomega.ContainSubstring(expectedOutput))
			})
		})
	}

	//WorkloadRunContext used for test vela svc deploy
	WorkloadRunContext = func(context string, cli string) bool {
		return ginkgo.Context(context, func() {
			ginkgo.It("should print successful creation information", func() {
				output, err := Exec(cli)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				gomega.Expect(output).To(gomega.ContainSubstring("deployed"))
			})
		})
	}

	WorkloadDeleteContext = func(context string, applicationName string) bool {
		return ginkgo.Context(context, func() {
			ginkgo.It("should print successful deletion information", func() {
				cli := fmt.Sprintf("vela delete %s", applicationName)
				output, err := Exec(cli)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				gomega.Expect(output).To(gomega.ContainSubstring("deleted from env"))
			})
		})
	}

	WorkloadCapabilityListContext = func() bool {
		return ginkgo.Context("list workload capabilities", func() {
			ginkgo.It("should sync capabilities from cluster before listing workload capabilities", func() {
				output, err := Exec("vela workloads")
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				gomega.Expect(output).To(gomega.ContainSubstring("webservice"))
			})
		})
	}

	TraitCapabilityListContext = func() bool {
		return ginkgo.Context("list traits capabilities", func() {
			ginkgo.It("should sync capabilities from cluster before listing trait capabilities", func() {
				output, err := Exec("vela traits")
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				gomega.Expect(output).To(gomega.ContainSubstring("scaler"))
			})
		})
	}
	// TraitManualScalerAttachContext used for test trait attach success
	TraitManualScalerAttachContext = func(context string, traitAlias string, applicationName string) bool {
		return ginkgo.Context(context, func() {
			ginkgo.It("should print successful attached information", func() {
				cli := fmt.Sprintf("vela %s %s", traitAlias, applicationName)
				output, err := LongTimeExec(cli, 180*time.Second)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				gomega.Expect(output).To(gomega.ContainSubstring("Adding " + traitAlias + " for app"))
				gomega.Expect(output).To(gomega.ContainSubstring("Checking Status"))
			})
		})
	}

	// ComponentListContext used for test vela svc ls
	ComponentListContext = func(context string, applicationName string, workloadType string, traitAlias string) bool {
		return ginkgo.Context("ls", func() {
			ginkgo.It("should list all applications", func() {
				output, err := Exec("vela ls")
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				gomega.Expect(output).To(gomega.ContainSubstring("SERVICE"))
				gomega.Expect(output).To(gomega.ContainSubstring(applicationName))
				gomega.Expect(output).To(gomega.ContainSubstring(workloadType))
				if traitAlias != "" {
					gomega.Expect(output).To(gomega.ContainSubstring(traitAlias))
				}
			})
		})
	}

	// APIEnvInitContext used for test api env
	APIEnvInitContext = func(context string, envMeta apis.Environment) bool {
		return ginkgo.Context("Post /envs/", func() {
			ginkgo.It("should create an env", func() {
				data, err := json.Marshal(&envMeta)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				resp, err := http.Post(util.URL("/envs/"), "application/json", strings.NewReader(string(data)))
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				defer resp.Body.Close()
				result, err := ioutil.ReadAll(resp.Body)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				var r apis.Response
				err = json.Unmarshal(result, &r)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				gomega.Expect(r.Code).Should(gomega.Equal(http.StatusOK))
				gomega.Expect(r.Data.(string)).To(gomega.ContainSubstring("created"))
			})
		})
	}
)

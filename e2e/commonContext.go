package e2e

import (
	ctx "context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"github.com/Netflix/go-expect"
	corev1alpha2 "github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
	"github.com/oam-dev/kubevela/pkg/server/apis"
	"github.com/oam-dev/kubevela/pkg/server/util"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
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

	SystemUpdateContext = func(context string) bool {
		return ginkgo.Context(context, func() {
			ginkgo.It("Synchronize workload/trait definitions from cluster", func() {
				output, err := Exec("vela system update")
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				gomega.Expect(output).To(gomega.ContainSubstring("workload definitions successfully synced"))
				gomega.Expect(output).To(gomega.ContainSubstring("trait definitions successfully synced"))
			})
		})
	}

	// RefreshContext used for test vela system update
	RefreshContext = func(context string) bool {
		return ginkgo.Context(context, func() {
			ginkgo.It("Sync commands from your Kubernetes cluster and locally cached them", func() {
				output, err := Exec("vela system update")
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				gomega.Expect(output).To(gomega.ContainSubstring("syncing workload definitions from cluster..."))
				gomega.Expect(output).To(gomega.ContainSubstring("sync"))
				gomega.Expect(output).To(gomega.ContainSubstring("successfully"))
				gomega.Expect(output).To(gomega.ContainSubstring("remove"))
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
				gomega.Expect(output).To(gomega.ContainSubstring("SUCCEED"))
			})
		})
	}

	WorkloadDeleteContext = func(context string, applicationName string) bool {
		return ginkgo.Context(context, func() {
			ginkgo.It("should print successful deletion information", func() {
				cli := fmt.Sprintf("vela app delete %s", applicationName)
				output, err := Exec(cli)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				gomega.Expect(output).To(gomega.ContainSubstring("delete apps succeed"))
			})
		})
	}

	// TraitManualScalerAttachContext used for test trait attach success
	TraitManualScalerAttachContext = func(context string, traitAlias string, applicationName string) bool {
		return ginkgo.Context(context, func() {
			ginkgo.It("should print successful attached information", func() {
				cli := fmt.Sprintf("vela %s %s", traitAlias, applicationName)
				output, err := Exec(cli)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				gomega.Expect(output).To(gomega.ContainSubstring("Adding " + traitAlias + " for app"))
				gomega.Expect(output).To(gomega.ContainSubstring("Succeeded!"))
			})
		})
	}

	// ComponentListContext used for test vela svc ls
	ComponentListContext = func(context string, applicationName string, traitAlias string) bool {
		return ginkgo.Context("ls", func() {
			ginkgo.It("should list all applications", func() {
				output, err := Exec("vela svc ls")
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				gomega.Expect(output).To(gomega.ContainSubstring("NAME"))
				gomega.Expect(output).To(gomega.ContainSubstring(applicationName))
				if traitAlias != "" {
					gomega.Expect(output).To(gomega.ContainSubstring(traitAlias))
				}
			})
		})
	}

	ApplicationStatusContext = func(context string, applicationName string, workloadType string) bool {
		return ginkgo.Context(context, func() {
			ginkgo.It("should get status for the application", func() {
				cli := fmt.Sprintf("vela app status %s", applicationName)
				output, err := Exec(cli)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				gomega.Expect(output).To(gomega.ContainSubstring(applicationName))
				// TODO(roywang) add more assertion to check health status
			})
		})
	}

	ApplicationCompStatusContext = func(context string, applicationName, workloadType, envName string) bool {
		return ginkgo.Context(context, func() {
			ginkgo.It("should get status of the service", func() {
				ginkgo.By("init new k8s client")
				k8sclient, err := newK8sClient()
				gomega.Expect(err).NotTo(gomega.HaveOccurred())

				ginkgo.By("check AppConfig reconciled ready")
				gomega.Eventually(func() int {
					appConfig := &corev1alpha2.ApplicationConfiguration{}
					_ = k8sclient.Get(ctx.Background(), client.ObjectKey{Name: applicationName, Namespace: "default"}, appConfig)
					return len(appConfig.Status.Workloads)
				}, 90*time.Second, 1*time.Second).ShouldNot(gomega.Equal(0))

				cli := fmt.Sprintf("vela svc status %s", applicationName)
				output, err := LongTimeExec(cli, 120*time.Second)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				gomega.Expect(output).To(gomega.ContainSubstring("Checking health status"))
				// TODO(zzxwill) need to check workloadType after app status is refined
			})
		})
	}

	ApplicationShowContext = func(context string, applicationName string, workloadType string) bool {
		return ginkgo.Context(context, func() {
			ginkgo.It("should show app information", func() {
				cli := fmt.Sprintf("vela app show %s", applicationName)
				output, err := Exec(cli)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				// TODO(zzxwill) need to check workloadType after app show is refined
				//gomega.Expect(output).To(gomega.ContainSubstring(workloadType))
				gomega.Expect(output).To(gomega.ContainSubstring(applicationName))
			})
		})
	}

	ApplicationExecContext = func(context string, appName string) bool {
		return ginkgo.Context(context, func() {
			ginkgo.It("should get output of exec /bin/ls", func() {
				gomega.Eventually(func() string {
					cli := fmt.Sprintf("vela exec %s -- /bin/ls ", appName)
					output, err := Exec(cli)
					gomega.Expect(err).NotTo(gomega.HaveOccurred())
					return output
				}, 90*time.Second, 5*time.Second).Should(gomega.ContainSubstring("bin"))
			})
		})
	}

	ApplicationPortForwardContext = func(context string, appName string) bool {
		return ginkgo.Context(context, func() {
			ginkgo.It("should get output of portward successfully", func() {
				cli := fmt.Sprintf("vela port-forward %s 8080:8080 ", appName)
				output, err := ExecAndTerminate(cli)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				gomega.Expect(output).To(gomega.ContainSubstring("Forward successfully"))
			})
		})
	}

	ApplicationInitIntercativeCliContext = func(context string, appName string, workloadType string) bool {
		return ginkgo.Context(context, func() {
			ginkgo.It("should init app through interactive questions", func() {
				cli := "vela init"
				output, err := InteractiveExec(cli, func(c *expect.Console) {
					data := []struct {
						q, a string
					}{
						{
							q: "Do you want to setup a domain for web service: ",
							a: "testdomain",
						},
						{
							q: "Provide an email for production certification: ",
							a: "test@mail",
						},
						{
							q: "What would you like to name your application: ",
							a: appName,
						},
						{
							q: "webservice",
							a: workloadType,
						},
						{
							q: "What would you name this webservice: ",
							a: "mysvc",
						},
						{
							q: "specify app image ",
							a: "nginx:latest",
						},
						{
							q: "specify port for container ",
							a: "8080",
						},
					}
					for _, qa := range data {
						_, err := c.ExpectString(qa.q)
						gomega.Expect(err).NotTo(gomega.HaveOccurred())
						_, err = c.SendLine(qa.a)
						gomega.Expect(err).NotTo(gomega.HaveOccurred())
					}
					_, _ = c.ExpectEOF()
				})
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				gomega.Expect(output).To(gomega.ContainSubstring("Initializing"))
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
				gomega.Expect(http.StatusOK).Should(gomega.Equal(r.Code))
				gomega.Expect(r.Data.(string)).To(gomega.ContainSubstring("created"))
			})
		})
	}
)

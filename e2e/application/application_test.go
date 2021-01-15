package e2e

import (
	context2 "context"
	"fmt"
	"time"

	"github.com/Netflix/go-expect"
	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/e2e"
)

var (
	envName                     = "env-application"
	workloadType                = "webservice"
	applicationName             = "app-basic"
	traitAlias                  = "scaler"
	appNameForInit              = "initmyapp"
	jsonAppFile                 = `{"name":"nginx-vela","services":{"nginx":{"type":"webservice","image":"nginx:1.9.4","port":80}}}`
	appbasicJsonAppFile         = `{"name":"app-basic","services":{"app-basic":{"type":"webservice","image":"nginx:1.9.4","port":80}}}`
	appbasicAddTraitJsonAppFile = `{"name":"app-basic","services":{"app-basic":{"type":"webservice","image":"nginx:1.9.4","port":80,"scaler":{"replicas":2}}}}`
)
var _ = ginkgo.Describe("Test Vela Init", func() {
	ApplicationInitIntercativeCliContext("init", appNameForInit, workloadType)
})

var _ = ginkgo.Describe("Test Vela Application", func() {
	e2e.JsonAppFileContext("json appfile apply", jsonAppFile)
	e2e.EnvSetContext("env set", "default")
	e2e.DeleteEnvFunc("env delete", envName)
	e2e.EnvInitContext("env init", envName)
	e2e.EnvSetContext("env set", envName)
	e2e.JsonAppFileContext("deploy app-basic", appbasicJsonAppFile)
	e2e.JsonAppFileContext("update app-basic, add scaler trait with replicas 2", appbasicAddTraitJsonAppFile)
	e2e.ComponentListContext("ls", applicationName, workloadType, traitAlias)
	ApplicationShowContext("show", applicationName, workloadType)
	ApplicationStatusContext("status", applicationName, workloadType)
	ApplicationStatusDeeplyContext("status", applicationName, workloadType, envName)
	ApplicationExecContext("exec -- COMMAND", applicationName)
	ApplicationPortForwardContext("port-forward", applicationName)
	e2e.WorkloadDeleteContext("delete", applicationName)
	e2e.WorkloadDeleteContext("delete", appNameForInit)
})

var ApplicationStatusContext = func(context string, applicationName string, workloadType string) bool {
	return ginkgo.Context(context, func() {
		ginkgo.It("should get status for the application", func() {
			cli := fmt.Sprintf("vela status %s", applicationName)
			output, err := e2e.Exec(cli)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(output).To(gomega.ContainSubstring(applicationName))
			// TODO(roywang) add more assertion to check health status
		})
	})
}

var ApplicationStatusDeeplyContext = func(context string, applicationName, workloadType, envName string) bool {
	return ginkgo.Context(context, func() {
		ginkgo.It("should get status of the service", func() {
			ginkgo.By("init new k8s client")
			k8sclient, err := e2e.NewK8sClient()
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			ginkgo.By("check AppConfig reconciled ready")
			gomega.Eventually(func() int {
				appConfig := &v1alpha2.ApplicationConfiguration{}
				_ = k8sclient.Get(context2.Background(), client.ObjectKey{Name: applicationName, Namespace: "default"}, appConfig)
				return len(appConfig.Status.Workloads)
			}, 90*time.Second, 1*time.Second).ShouldNot(gomega.Equal(0))

			cli := fmt.Sprintf("vela status %s", applicationName)
			output, err := e2e.LongTimeExec(cli, 120*time.Second)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(output).To(gomega.ContainSubstring("Checking health status"))
			// TODO(zzxwill) need to check workloadType after app status is refined
		})
	})
}

var ApplicationShowContext = func(context string, applicationName string, workloadType string) bool {
	return ginkgo.Context(context, func() {
		ginkgo.It("should show app information", func() {
			cli := fmt.Sprintf("vela show %s", applicationName)
			output, err := e2e.Exec(cli)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			// TODO(zzxwill) need to check workloadType after app show is refined
			//gomega.Expect(output).To(gomega.ContainSubstring(workloadType))
			gomega.Expect(output).To(gomega.ContainSubstring(applicationName))
		})
	})
}

var ApplicationExecContext = func(context string, appName string) bool {
	return ginkgo.Context(context, func() {
		ginkgo.It("should get output of exec /bin/ls", func() {
			gomega.Eventually(func() string {
				cli := fmt.Sprintf("vela exec %s -- /bin/ls ", appName)
				output, err := e2e.Exec(cli)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return output
			}, 90*time.Second, 5*time.Second).Should(gomega.ContainSubstring("bin"))
		})
	})
}

var ApplicationPortForwardContext = func(context string, appName string) bool {
	return ginkgo.Context(context, func() {
		ginkgo.It("should get output of portward successfully", func() {
			cli := fmt.Sprintf("vela port-forward %s 8080:8080 ", appName)
			output, err := e2e.ExecAndTerminate(cli)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(output).To(gomega.ContainSubstring("Forward successfully"))
		})
	})
}

var ApplicationInitIntercativeCliContext = func(context string, appName string, workloadType string) bool {
	return ginkgo.Context(context, func() {
		ginkgo.It("should init app through interactive questions", func() {
			cli := "vela init"
			output, err := e2e.InteractiveExec(cli, func(c *expect.Console) {
				data := []struct {
					q, a string
				}{
					{
						q: "What is the domain of your application service (optional): ",
						a: "testdomain",
					},
					{
						q: "What is your email (optional, used to generate certification): ",
						a: "test@mail",
					},
					{
						q: "What would you like to name your application (required): ",
						a: appName,
					},
					{
						q: "webservice",
						a: workloadType,
					},
					{
						q: "What would you like to name this webservice (required): ",
						a: "mysvc",
					},
					{
						q: "Which image would you like to use for your service ",
						a: "nginx:latest",
					},
					{
						q: "Which port do you want customer traffic sent to ",
						a: "",
					},
					{
						q: "Number of CPU units for the service, like `0.5` (0.5 CPU core), `1` (1 CPU core) (optional):",
						a: "0.5",
					},
				}
				for _, qa := range data {
					_, err := c.ExpectString(qa.q)
					gomega.Expect(err).NotTo(gomega.HaveOccurred())
					_, err = c.SendLine(qa.a)
					gomega.Expect(err).NotTo(gomega.HaveOccurred())
				}
				_, err := c.ExpectEOF()
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
			})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(output).To(gomega.ContainSubstring("Checking Status"))
		})
	})
}

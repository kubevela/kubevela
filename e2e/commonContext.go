package e2e

import (
	"fmt"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
)

var (
	// Refresh
	RefreshContext = func(context string) bool {
		return ginkgo.Context(context, func() {
			ginkgo.It("Sync commands from your Kubernetes cluster and locally cached them", func() {
				output, err := Exec("vela refresh")
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				gomega.Expect(output).To(gomega.ContainSubstring("syncing workload definitions from cluster..."))
				gomega.Expect(output).To(gomega.ContainSubstring("successfully synced"))
			})
		})
	}

	// Env
	EnvInitContext = func(context string, envName string) bool {
		return ginkgo.Context(context, func() {
			ginkgo.It("should print env initiation successful message", func() {
				cli := fmt.Sprintf("vela env:init %s --namespace %s", envName, envName)
				output, err := Exec(cli)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				expectedOutput := fmt.Sprintf("Create env succeed, current env is %s", envName)
				gomega.Expect(output).To(gomega.ContainSubstring(expectedOutput))
			})
		})
	}

	EnvShowContext = func(context string, envName string) bool {
		return ginkgo.Context(context, func() {
			ginkgo.It("should show detailed env message", func() {
				cli := fmt.Sprintf("vela env %s", envName)
				output, err := Exec(cli)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				expectedOutput := fmt.Sprintf("%s\t%s", envName, envName)
				gomega.Expect(output).To(gomega.ContainSubstring("NAME"))
				gomega.Expect(output).To(gomega.ContainSubstring("NAMESPACE"))
				gomega.Expect(output).To(gomega.ContainSubstring(expectedOutput))
			})
		})
	}

	EnvSwitchContext = func(context string, envName string) bool {
		return ginkgo.Context(context, func() {
			ginkgo.It("should show env switch message", func() {
				cli := fmt.Sprintf("vela env:sw %s", envName)
				output, err := Exec(cli)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				expectedOutput := fmt.Sprintf("Switch env succeed, current env is %s", envName)
				gomega.Expect(output).To(gomega.ContainSubstring(expectedOutput))
			})
		})
	}

	EnvDeleteContext = func(context string, envName string) bool {
		return ginkgo.Context(context, func() {
			ginkgo.It("should delete all envs", func() {
				cli := fmt.Sprintf("vela env:delete %s", envName)
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
				cli := fmt.Sprintf("vela env:delete %s", envName)
				output, err := Exec(cli)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				expectedOutput := fmt.Sprintf("Error: you can't delete current using env %s", envName)
				gomega.Expect(output).To(gomega.ContainSubstring(expectedOutput))
			})
		})
	}

	//Workload
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
				cli := fmt.Sprintf("vela app:delete %s", applicationName)
				output, err := Exec(cli)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				gomega.Expect(output).To(gomega.ContainSubstring("DELETE SUCCEED"))
			})
		})
	}
)

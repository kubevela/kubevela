package e2e

import (
	"fmt"

	"github.com/cloud-native-application/rudrx/e2e"
	"github.com/onsi/ginkgo"
)

var (
	//TODO(zzxwill) Need to change env name after [issue#82](https://github.com/cloud-native-application/RudrX/issues/82) is fixed.
	envName         = "default"
	applicationName = "app-testworkloadrun-basic"
)

var _ = ginkgo.Describe("Workload", func() {
	e2e.RefreshContext("refresh")
	e2e.EnvInitContext("env init", envName)
	e2e.EnvSwitchContext("env switch", envName)
	e2e.WorkloadRunContext("run", fmt.Sprintf("vela containerized:run %s -p 80 --image nginx:1.9.4", applicationName))
	e2e.WorkloadDeleteContext("delete", applicationName)
})

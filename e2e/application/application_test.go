package e2e

import (
	"fmt"

	"github.com/cloud-native-application/rudrx/e2e"
	"github.com/onsi/ginkgo"
)

var (
	envName         = "env-application"
	workloadType    = "containerized"
	applicationName = "app-basic"
	traitAlias      = "scale"
)

var _ = ginkgo.Describe("Application", func() {
	e2e.EnvInitContext("env init", envName)
	e2e.EnvShowContext("env show", envName)
	e2e.EnvSwitchContext("env switch", envName)
	e2e.WorkloadRunContext("run", fmt.Sprintf("vela comp run -t %s %s -p 80 --image nginx:1.9.4",
		workloadType, applicationName))
	e2e.ApplicationListContext("app ls", applicationName, "")
	e2e.TraitManualScalerAttachContext("vela attach trait", traitAlias, applicationName)
	//e2e.ApplicationListContext("app ls", applicationName, traitAlias)
	e2e.ApplicationShowContext("app show", applicationName, workloadType)
	e2e.ApplicationStatusContext("app status", applicationName, workloadType)
	e2e.WorkloadDeleteContext("delete", applicationName)
})

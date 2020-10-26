package e2e

import (
	"fmt"

	"github.com/onsi/ginkgo"

	"github.com/oam-dev/kubevela/e2e"
)

var (
	envName         = "env-application"
	workloadType    = "webservice"
	applicationName = "app-basic"
	traitAlias      = "scale"
	appNameForInit  = "initmyapp"
)

var _ = ginkgo.Describe("Application", func() {
	e2e.EnvSetContext("env set", "default")
	e2e.DeleteEnvFunc("env delete", envName)
	e2e.EnvInitContext("env init", envName)
	e2e.EnvSetContext("env set", envName)
	e2e.WorkloadRunContext("deploy", fmt.Sprintf("vela comp deploy -t %s %s -p 80 --image nginx:1.9.4",
		workloadType, applicationName))
	e2e.ComponentListContext("comp ls", applicationName, "")
	e2e.TraitManualScalerAttachContext("vela attach scale trait", traitAlias, applicationName)
	e2e.ApplicationShowContext("app show", applicationName, workloadType)
	e2e.ApplicationStatusContext("app status", applicationName, workloadType)
	e2e.ApplicationCompStatusContext("comp status", applicationName, workloadType, envName)
	e2e.ApplicationExecContext("exec -- COMMAND", applicationName)
	e2e.ApplicationInitIntercativeCliContext("init", appNameForInit, workloadType)
	e2e.WorkloadDeleteContext("delete", applicationName)
	e2e.WorkloadDeleteContext("delete", appNameForInit)
})

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
	traitAlias      = "scaler"
	appNameForInit  = "initmyapp"
	jsonAppFile 	= `{"name":"nginx-vela","services":{"nginx":{"type":"webservice","image":"nginx:1.9.4","port":80}}}`
)

var _ = ginkgo.Describe("Application", func() {
	e2e.JsonAppFileContext("json appfile apply", jsonAppFile)
	e2e.EnvSetContext("env set", "default")
	e2e.DeleteEnvFunc("env delete", envName)
	e2e.EnvInitContext("env init", envName)
	e2e.EnvSetContext("env set", envName)
	e2e.WorkloadRunContext("deploy", fmt.Sprintf("vela svc deploy -t %s %s -p 80 --image nginx:1.9.4",
		workloadType, applicationName))
	e2e.TraitManualScalerAttachContext("vela attach scaler trait", traitAlias, applicationName)
	e2e.ComponentListContext("ls", applicationName, workloadType, traitAlias)
	e2e.ApplicationShowContext("show", applicationName, workloadType)
	e2e.ApplicationStatusContext("status", applicationName, workloadType)
	e2e.ApplicationStatusDeeplyContext("status", applicationName, workloadType, envName)
	e2e.ApplicationExecContext("exec -- COMMAND", applicationName)
	e2e.ApplicationPortForwardContext("port-forward", applicationName)
	e2e.ApplicationInitIntercativeCliContext("init", appNameForInit, workloadType)
	e2e.WorkloadDeleteContext("delete", applicationName)
	e2e.WorkloadDeleteContext("delete", appNameForInit)
})

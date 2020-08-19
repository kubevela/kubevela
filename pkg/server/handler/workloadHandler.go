package handler

import (
	"github.com/cloud-native-application/rudrx/api/types"
	"github.com/cloud-native-application/rudrx/pkg/oam"
	"github.com/cloud-native-application/rudrx/pkg/plugins"
	"github.com/spf13/pflag"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/cloud-native-application/rudrx/pkg/server/apis"
	"github.com/cloud-native-application/rudrx/pkg/server/util"
	"github.com/gin-gonic/gin"
)

// Workload related handlers
func CreateWorkload(c *gin.Context) {
	kubeClient := c.MustGet("KubeClient")
	var body apis.WorkloadRunBody
	if err := c.ShouldBindJSON(&body); err != nil {
		util.HandleError(c, util.InvalidArgument, "the workload run request body is invalid")
		return
	}
	fs := pflag.NewFlagSet("workload", pflag.ContinueOnError)
	for _, f := range body.Flags {
		fs.String(f.Name, f.Value, "")
	}
	evnName := body.EnvName
	var template types.Capability

	template, err := plugins.LoadCapabilityByName(body.WorkloadType)
	appObj, err := oam.BaseComplete(evnName, body.WorkloadName, body.AppGroup, fs, template)
	if err != nil {
		util.HandleError(c, util.StatusInternalServerError, err.Error())
		return
	}
	env, err := oam.GetEnvByName(evnName)
	if err != nil {
		util.HandleError(c, util.StatusInternalServerError, err.Error())
		return
	}
	msg, err := oam.BaseRun(body.Staging, appObj, kubeClient.(client.Client), env)
	if err != nil {
		util.HandleError(c, util.StatusInternalServerError, err.Error())
		return
	}
	util.AssembleResponse(c, msg, err)
}

func UpdateWorkload(c *gin.Context) {
}

func GetWorkload(c *gin.Context) {
}

func ListWorkload(c *gin.Context) {
}

func DeleteWorkload(c *gin.Context) {
}

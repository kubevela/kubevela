package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/spf13/pflag"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/api/types"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/plugins"
	"github.com/oam-dev/kubevela/pkg/server/apis"
	"github.com/oam-dev/kubevela/pkg/server/util"
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

	appObj, err := oam.BaseComplete(evnName, body.WorkloadName, body.AppName, fs, body.WorkloadType)
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
	if len(body.Traits) == 0 {
		util.AssembleResponse(c, msg, err)
	} else {
		for _, t := range body.Traits {
			t.AppName = body.AppName
			t.ComponentName = body.WorkloadName
			msg, err = oam.AttachTrait(c, t)
			if err != nil {
				util.HandleError(c, util.StatusInternalServerError, err.Error())
				return
			}
		}
		util.AssembleResponse(c, msg, err)
	}
}

func UpdateWorkload(c *gin.Context) {
}

func GetWorkload(c *gin.Context) {
	var workloadType = c.Param("workloadName")
	var capability types.Capability
	var err error

	if capability, err = plugins.GetInstalledCapabilityWithCapAlias(types.TypeWorkload, workloadType); err != nil {
		util.HandleError(c, util.StatusInternalServerError, err)
		return
	}
	util.AssembleResponse(c, capability, err)
}

func ListWorkload(c *gin.Context) {
	var workloadDefinitionList []apis.WorkloadMeta
	workloads, err := plugins.LoadInstalledCapabilityWithType(types.TypeWorkload)
	if err != nil {
		util.HandleError(c, util.StatusInternalServerError, err)
		return
	}
	for _, w := range workloads {
		workloadDefinitionList = append(workloadDefinitionList, apis.WorkloadMeta{
			Name:       w.Name,
			Parameters: w.Parameters,
			AppliesTo:  w.AppliesTo,
		})
	}
	util.AssembleResponse(c, workloadDefinitionList, err)
}

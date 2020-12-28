package server

import (
	"os"

	"github.com/gin-gonic/gin"
	"github.com/spf13/pflag"

	"github.com/oam-dev/kubevela/apis/types"
	cmdutil "github.com/oam-dev/kubevela/pkg/commands/util"
	"github.com/oam-dev/kubevela/pkg/plugins"
	"github.com/oam-dev/kubevela/pkg/server/apis"
	"github.com/oam-dev/kubevela/pkg/server/util"
	"github.com/oam-dev/kubevela/pkg/serverlib"
	env2 "github.com/oam-dev/kubevela/pkg/utils/env"
)

// CreateWorkload creates a workload
func (s *APIServer) CreateWorkload(c *gin.Context) {
	var body apis.WorkloadRunBody
	if err := c.ShouldBindJSON(&body); err != nil {
		util.HandleError(c, util.InvalidArgument, "the workload run request body is invalid")
		return
	}
	fs := pflag.NewFlagSet("workload", pflag.ContinueOnError)
	for _, f := range body.Flags {
		fs.String(f.Name, f.Value, "")
	}
	envName := body.EnvName

	appObj, err := serverlib.BaseComplete(envName, body.WorkloadName, body.AppName, fs, body.WorkloadType)
	if err != nil {
		util.HandleError(c, util.StatusInternalServerError, err.Error())
		return
	}
	env, err := env2.GetEnvByName(envName)
	if err != nil {
		util.HandleError(c, util.StatusInternalServerError, err.Error())
		return
	}
	io := cmdutil.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr}
	msg, err := serverlib.BaseRun(body.Staging, appObj, s.KubeClient, env, io)
	if err != nil {
		util.HandleError(c, util.StatusInternalServerError, err.Error())
		return
	}
	if len(body.Traits) == 0 {
		util.AssembleResponse(c, msg, err)
		return
	}
	for _, t := range body.Traits {
		t.AppName = body.AppName
		t.ComponentName = body.WorkloadName
		msg, err = s.DoAttachTrait(c, t)
		if err != nil {
			util.HandleError(c, util.StatusInternalServerError, err.Error())
			return
		}
	}
	util.AssembleResponse(c, msg, err)
}

// UpdateWorkload updates a workload
func (s *APIServer) UpdateWorkload(c *gin.Context) {
}

// GetWorkload gets a workload by name
func (s *APIServer) GetWorkload(c *gin.Context) {
	var workloadType = c.Param("workloadName")
	var capability types.Capability
	var err error

	if capability, err = plugins.GetInstalledCapabilityWithCapName(types.TypeWorkload, workloadType); err != nil {
		util.HandleError(c, util.StatusInternalServerError, err)
		return
	}
	util.AssembleResponse(c, capability, err)
}

// ListWorkload lists all workloads in the cluster
func (s *APIServer) ListWorkload(c *gin.Context) {
	var workloadDefinitionList []apis.WorkloadMeta
	workloads, err := plugins.LoadInstalledCapabilityWithType(types.TypeWorkload)
	if err != nil {
		util.HandleError(c, util.StatusInternalServerError, err)
		return
	}
	for _, w := range workloads {
		workloadDefinitionList = append(workloadDefinitionList, apis.WorkloadMeta{
			Name:        w.Name,
			Parameters:  w.Parameters,
			Description: w.Description,
		})
	}
	util.AssembleResponse(c, workloadDefinitionList, err)
}

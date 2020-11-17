package server

import (
	"os"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/spf13/pflag"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/oam-dev/kubevela/api/types"
	"github.com/oam-dev/kubevela/pkg/application"
	util2 "github.com/oam-dev/kubevela/pkg/commands/util"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/plugins"
	"github.com/oam-dev/kubevela/pkg/server/apis"
	"github.com/oam-dev/kubevela/pkg/server/util"
	env2 "github.com/oam-dev/kubevela/pkg/utils/env"
)

// Trait related handlers
func (s *APIServer) AttachTrait(c *gin.Context) {
	var body apis.TraitBody
	body.EnvName = c.Param("envName")
	body.AppName = c.Param("appName")
	body.ComponentName = c.Param("compName")

	if err := c.ShouldBindJSON(&body); err != nil {
		util.HandleError(c, util.InvalidArgument, "the trait attach request body is invalid")
		return
	}
	ctrl.Log.Info("request parameters body:", "body", body)
	msg, err := s.DoAttachTrait(c, body)
	if err != nil {
		util.HandleError(c, util.StatusInternalServerError, err.Error())
		return
	}
	util.AssembleResponse(c, msg, nil)
}

func (s *APIServer) GetTrait(c *gin.Context) {
	var traitType = c.Param("traitName")
	var workloadType string
	var capability types.Capability
	var err error

	if capability, err = oam.GetTraitDefinition(&workloadType, traitType); err != nil {
		util.HandleError(c, util.StatusInternalServerError, err)
		return
	}
	util.AssembleResponse(c, capability, err)
}

func (s *APIServer) ListTrait(c *gin.Context) {
	var traitList []types.Capability
	var workloadName string
	var err error
	if traitList, err = oam.ListTraitDefinitions(&workloadName); err != nil {
		util.HandleError(c, util.StatusInternalServerError, err)
		return
	}
	util.AssembleResponse(c, traitList, err)
}

func (s *APIServer) DetachTrait(c *gin.Context) {
	envName := c.Param("envName")
	traitType := c.Param("traitName")
	componentName := c.Param("compName")
	applicationName := c.Param("appName")

	var staging = false
	var err error
	if stagingStr := c.Param("staging"); stagingStr != "" {
		if staging, err = strconv.ParseBool(stagingStr); err != nil {
			util.HandleError(c, util.StatusInternalServerError, err.Error())
			return
		}
	}
	msg, err := s.DoDetachTrait(c, envName, traitType, componentName, applicationName, staging)
	if err != nil {
		util.HandleError(c, util.StatusInternalServerError, err.Error())
		return
	}
	util.AssembleResponse(c, msg, nil)
}

func (s *APIServer) DoAttachTrait(c *gin.Context, body apis.TraitBody) (string, error) {
	// Prepare
	var appObj *application.Application
	fs := pflag.NewFlagSet("trait", pflag.ContinueOnError)
	for _, f := range body.Flags {
		fs.String(f.Name, f.Value, "")
	}
	var staging = false
	var err error
	if body.Staging != "" {
		staging, err = strconv.ParseBool(body.Staging)
		if err != nil {
			return "", err
		}
	}
	traitAlias := body.Name
	template, err := plugins.GetInstalledCapabilityWithCapAlias(types.TypeTrait, traitAlias)
	if err != nil {
		return "", err
	}
	// Run step
	env, err := env2.GetEnvByName(body.EnvName)
	if err != nil {
		return "", err
	}

	appObj, err = oam.AddOrUpdateTrait(env, body.AppName, body.ComponentName, fs, template)
	if err != nil {
		return "", err
	}
	io := util2.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr}
	return oam.TraitOperationRun(c, s.KubeClient, env, appObj, staging, io)
}

func (s *APIServer) DoDetachTrait(c *gin.Context, envName string, traitType string, componentName string, appName string, staging bool) (string, error) {
	var appObj *application.Application
	var err error
	if appName == "" {
		appName = componentName
	}
	if appObj, err = oam.PrepareDetachTrait(envName, traitType, componentName, appName); err != nil {
		return "", err
	}
	// Run
	env, err := env2.GetEnvByName(envName)
	if err != nil {
		return "", err
	}
	io := util2.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr}
	return oam.TraitOperationRun(c, s.KubeClient, env, appObj, staging, io)
}

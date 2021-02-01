package server

import (
	"fmt"
	"os"

	"github.com/gin-gonic/gin"

	"github.com/oam-dev/kubevela/pkg/appfile/api"
	cmdutil "github.com/oam-dev/kubevela/pkg/commands/util"
	"github.com/oam-dev/kubevela/pkg/server/util"
	"github.com/oam-dev/kubevela/pkg/serverlib"
	"github.com/oam-dev/kubevela/pkg/utils/env"
)

// UpdateApps is placeholder for updating applications
func (s *APIServer) UpdateApps(c *gin.Context) {
}

// GetApp requests an application by the namespaced name in the gin.Context
func (s *APIServer) GetApp(c *gin.Context) {
	envName := c.Param("envName")
	envMeta, err := env.GetEnvByName(envName)
	if err != nil {
		util.HandleError(c, util.StatusInternalServerError, err)
		return
	}
	namespace := envMeta.Namespace
	appName := c.Param("appName")
	ctx := util.GetContext(c)
	applicationMeta, err := serverlib.RetrieveApplicationStatusByName(ctx, s.KubeClient, appName, namespace)
	if err != nil {
		util.HandleError(c, util.StatusInternalServerError, err)
		return
	}
	util.AssembleResponse(c, applicationMeta, nil)
}

// ListApps requests a list of application by the namespace in the gin.Context
// @tags applications
// @ID ListApplications
// @Summary list all applications
// @Param envName path string true "environment name"
// @Success 200 {object} apis.Response{code=int,data=[]apis.ApplicationMeta}
// @Failure 500 {object} apis.Response{code=int,data=string}
// @Router /envs/{envName}/apps [get]
func (s *APIServer) ListApps(c *gin.Context) {
	envName := c.Param("envName")
	envMeta, err := env.GetEnvByName(envName)
	if err != nil {
		util.HandleError(c, util.StatusInternalServerError, err)
		return
	}
	namespace := envMeta.Namespace

	ctx := util.GetContext(c)
	applicationMetaList, err := serverlib.ListApplications(ctx, s.KubeClient, serverlib.Option{Namespace: namespace})
	if err != nil {
		util.HandleError(c, util.StatusInternalServerError, err.Error())
		return
	}
	util.AssembleResponse(c, applicationMetaList, nil)
}

// DeleteApps deletes an application by the namespaced name in the gin.Context
func (s *APIServer) DeleteApps(c *gin.Context) {
	envName := c.Param("envName")
	envMeta, err := env.GetEnvByName(envName)
	if err != nil {
		util.HandleError(c, util.StatusInternalServerError, err)
		return
	}
	appName := c.Param("appName")

	o := serverlib.DeleteOptions{
		Client:  s.KubeClient,
		Env:     envMeta,
		AppName: appName,
	}
	message, err := o.DeleteApp()
	util.AssembleResponse(c, message, err)
}

// CreateApplication creates an application
// @tags applications
// @ID CreateApplication
// @Summary creates an application
// @Param envName path string true "environment name"
// @Param body body appfile.AppFile true "application parameters"
// @Success 200 {object} apis.Response{code=int,data=string}
// @Failure 500 {object} apis.Response{code=int,data=string}
// @Router /envs/{envName}/apps [post]
func (s *APIServer) CreateApplication(c *gin.Context) {
	var body api.AppFile
	if err := c.ShouldBindJSON(&body); err != nil {
		util.HandleError(c, util.InvalidArgument, "the application creation request body is invalid")
		return
	}
	env, err := env.GetEnvByName(c.Param("envName"))
	if err != nil {
		util.HandleError(c, util.StatusInternalServerError, err.Error())
		return
	}
	ioStream := cmdutil.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr}
	o := &serverlib.AppfileOptions{
		Kubecli: s.KubeClient,
		IO:      ioStream,
		Env:     env,
	}
	buildResult, data, err := o.ExportFromAppFile(&body, false)
	if err != nil {
		util.HandleError(c, util.StatusInternalServerError, err.Error())
		return
	}
	err = o.BaseAppFileRun(buildResult, data, s.dm)
	if err != nil {
		util.HandleError(c, util.StatusInternalServerError, err.Error())
		return
	}
	msg := fmt.Sprintf("application %s is successfully created", body.Name)
	util.AssembleResponse(c, msg, nil)
}

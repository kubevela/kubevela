package server

import (
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/server/util"
	"github.com/oam-dev/kubevela/pkg/utils/env"

	"github.com/gin-gonic/gin"
)

// UpdateApps is placeholder for updating applications
func (s *APIServer) UpdateApps(c *gin.Context) {
}

// GetApp requests an application by the namespacedname in the gin.Context
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
	applicationMeta, err := oam.RetrieveApplicationStatusByName(ctx, s.KubeClient, appName, namespace)
	if err != nil {
		util.HandleError(c, util.StatusInternalServerError, err)
		return
	}
	util.AssembleResponse(c, applicationMeta, nil)
}

// ListApps requests a list of application by the namespace in the gin.Context
func (s *APIServer) ListApps(c *gin.Context) {
	envName := c.Param("envName")
	envMeta, err := env.GetEnvByName(envName)
	if err != nil {
		util.HandleError(c, util.StatusInternalServerError, err)
		return
	}
	namespace := envMeta.Namespace

	ctx := util.GetContext(c)
	applicationMetaList, err := oam.ListApplications(ctx, s.KubeClient, oam.Option{Namespace: namespace})
	if err != nil {
		util.HandleError(c, util.StatusInternalServerError, err.Error())
		return
	}
	util.AssembleResponse(c, applicationMetaList, nil)
}

// DeleteApps deletes an application by the namespacedname in the gin.Context
func (s *APIServer) DeleteApps(c *gin.Context) {
	envName := c.Param("envName")
	envMeta, err := env.GetEnvByName(envName)
	if err != nil {
		util.HandleError(c, util.StatusInternalServerError, err)
		return
	}
	appName := c.Param("appName")

	o := oam.DeleteOptions{
		Client:  s.KubeClient,
		Env:     envMeta,
		AppName: appName,
	}
	message, err := o.DeleteApp()
	util.AssembleResponse(c, message, err)
}

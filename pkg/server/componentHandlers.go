//nolint:golint
package server

import (
	"os"

	"github.com/gin-gonic/gin"

	cmdutil "github.com/oam-dev/kubevela/pkg/commands/util"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/server/util"
	"github.com/oam-dev/kubevela/pkg/utils/env"
)

func (s *APIServer) GetComponent(c *gin.Context) {
	envName := c.Param("envName")
	envMeta, err := env.GetEnvByName(envName)
	if err != nil {
		util.HandleError(c, util.StatusInternalServerError, err)
		return
	}
	namespace := envMeta.Namespace
	applicationName := c.Param("appName")
	componentName := c.Param("compName")
	ctx := util.GetContext(c)
	componentMeta, err := oam.RetrieveComponent(ctx, s.KubeClient, applicationName, componentName, namespace)
	if err != nil {
		util.HandleError(c, util.StatusInternalServerError, err)
		return
	}
	util.AssembleResponse(c, componentMeta, nil)
}

func (s *APIServer) DeleteComponent(c *gin.Context) {
	envName := c.Param("envName")
	envMeta, err := env.GetEnvByName(envName)
	if err != nil {
		util.HandleError(c, util.StatusInternalServerError, err)
		return
	}
	appName := c.Param("appName")
	componentName := c.Param("compName")

	o := oam.DeleteOptions{
		Client:   s.KubeClient,
		Env:      envMeta,
		AppName:  appName,
		CompName: componentName}

	message, err := o.DeleteComponent(
		cmdutil.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr})
	util.AssembleResponse(c, message, err)
}

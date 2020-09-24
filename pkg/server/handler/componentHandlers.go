package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/server/util"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func GetComponent(c *gin.Context) {
	kubeClient := c.MustGet("KubeClient")
	envName := c.Param("envName")
	envMeta, err := oam.GetEnvByName(envName)
	if err != nil {
		util.HandleError(c, util.StatusInternalServerError, err)
		return
	}
	namespace := envMeta.Namespace
	applicationName := c.Param("appName")
	componentName := c.Param("compName")
	ctx := util.GetContext(c)
	componentMeta, err := oam.RetrieveComponent(ctx, kubeClient.(client.Client), applicationName, componentName, namespace)
	if err != nil {
		util.HandleError(c, util.StatusInternalServerError, err)
		return
	}
	util.AssembleResponse(c, componentMeta, nil)
}

func DeleteComponent(c *gin.Context) {
	kubeClient := c.MustGet("KubeClient")
	envName := c.Param("envName")
	envMeta, err := oam.GetEnvByName(envName)
	if err != nil {
		util.HandleError(c, util.StatusInternalServerError, err)
		return
	}
	appName := c.Param("appName")
	componentName := c.Param("compName")

	o := oam.DeleteOptions{
		Client:   kubeClient.(client.Client),
		Env:      envMeta,
		AppName:  appName,
		CompName: componentName}
	message, err := o.DeleteComponent()
	util.AssembleResponse(c, message, err)
}

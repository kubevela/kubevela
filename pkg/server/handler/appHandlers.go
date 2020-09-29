package handler

import (
	"github.com/oam-dev/kubevela/pkg/server/util"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/gin-gonic/gin"
	"github.com/oam-dev/kubevela/pkg/oam"
)

func UpdateApps(c *gin.Context) {
}

func GetApp(c *gin.Context) {
	kubeClient := c.MustGet("KubeClient")
	envName := c.Param("envName")
	envMeta, err := oam.GetEnvByName(envName)
	if err != nil {
		util.HandleError(c, util.StatusInternalServerError, err)
		return
	}
	namespace := envMeta.Namespace
	appName := c.Param("appName")
	ctx := util.GetContext(c)
	applicationMeta, err := oam.RetrieveApplicationStatusByName(ctx, kubeClient.(client.Client), appName, namespace)
	if err != nil {
		util.HandleError(c, util.StatusInternalServerError, err)
		return
	}
	util.AssembleResponse(c, applicationMeta, nil)
}

func ListApps(c *gin.Context) {
	kubeClient := c.MustGet("KubeClient")
	envName := c.Param("envName")
	envMeta, err := oam.GetEnvByName(envName)
	if err != nil {
		util.HandleError(c, util.StatusInternalServerError, err)
		return
	}
	namespace := envMeta.Namespace

	ctx := util.GetContext(c)
	applicationMetaList, err := oam.ListApplications(ctx, kubeClient.(client.Client), oam.Option{Namespace: namespace})
	if err != nil {
		util.HandleError(c, util.StatusInternalServerError, err.Error())
		return
	}
	util.AssembleResponse(c, applicationMetaList, nil)
}

func DeleteApps(c *gin.Context) {
	kubeClient := c.MustGet("KubeClient")
	envName := c.Param("envName")
	envMeta, err := oam.GetEnvByName(envName)
	if err != nil {
		util.HandleError(c, util.StatusInternalServerError, err)
		return
	}
	appName := c.Param("appName")

	o := oam.DeleteOptions{
		Client:  kubeClient.(client.Client),
		Env:     envMeta,
		AppName: appName,
	}
	message, err := o.DeleteApp()
	util.AssembleResponse(c, message, err)
}

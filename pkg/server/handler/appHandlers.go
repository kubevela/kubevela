package handler

import (
	"github.com/cloud-native-application/rudrx/pkg/server/util"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/cloud-native-application/rudrx/pkg/oam"
	"github.com/gin-gonic/gin"
)

const querymodeKey = "appQuerymode"

// Apps related handlers
func CreateApps(c *gin.Context) {

}

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
	applicationStatus, err := oam.RetrieveApplicationStatusByName(ctx, kubeClient.(client.Client), appName, namespace)
	if err != nil {
		util.HandleError(c, util.StatusInternalServerError, err)
		return
	}
	util.AssembleResponse(c, applicationStatus, nil)
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
	applicationMetaList, err := oam.ListComponents(ctx, kubeClient.(client.Client), oam.Option{Namespace: namespace})
	if err != nil {
		util.HandleError(c, util.StatusInternalServerError, err.Error())
		return
	}
	util.AssembleResponse(c, applicationMetaList, nil)
}

func DeleteApps(c *gin.Context) {
}

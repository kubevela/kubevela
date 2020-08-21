package handler

import (
	"net/http"

	"github.com/cloud-native-application/rudrx/pkg/server/apis"

	"github.com/cloud-native-application/rudrx/pkg/server/util"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/cloud-native-application/rudrx/pkg/oam"
	"github.com/gin-gonic/gin"
	ctrl "sigs.k8s.io/controller-runtime"
)

const querymodeKey = "appQuerymode"

// Apps related handlers
func CreateApps(c *gin.Context) {

}

func UpdateApps(c *gin.Context) {
}

func GetApps(c *gin.Context) {
	envName := c.Param("envName")
	appName := c.Param("appName")
	queryMode, found := c.GetQuery(querymodeKey)
	if !found {
		panic("no repoUrl in update")
	}
	ctrl.Log.Info("Get an application request for", "envName", envName, "appName", appName, "queryMdoe", queryMode)
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
	applicationMetaList, err := oam.RetrieveApplicationsByName(ctx, kubeClient.(client.Client), "", namespace)
	if err != nil {
		util.HandleError(c, util.StatusInternalServerError, err.Error())
		return
	}
	resp := apis.Response{
		Code: http.StatusOK,
		Data: applicationMetaList,
	}
	c.JSON(http.StatusOK, resp)
}

func DeleteApps(c *gin.Context) {
}

package handler

import (
	"context"
	"net/http"

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
	namespace := c.Param("envName")
	ctx := context.Background()
	applicationMetaList, err := oam.RetrieveApplicationsByName(ctx, kubeClient.(client.Client), "", namespace)
	if err != nil {
		c.JSON(http.StatusInternalServerError, []oam.ApplicationMeta{})
	}
	c.JSON(http.StatusOK, applicationMetaList)
}

func DeleteApps(c *gin.Context) {
}

package handler

import (
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
}

func DeleteApps(c *gin.Context) {
}

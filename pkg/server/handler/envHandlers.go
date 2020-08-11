package handler

import (
	"github.com/gin-gonic/gin"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/cloud-native-application/rudrx/pkg/server/apis"
	"github.com/cloud-native-application/rudrx/pkg/server/util"
)

// ENV related handlers
func CreateEnv(c *gin.Context) {
	var envConfig apis.Environment
	if err := c.ShouldBindJSON(&envConfig); err != nil {
		util.SetErrorAndAbort(c, util.InvalidArgument, "the create environment request body is invalid")
	}
	ctrl.Log.Info("Get Create Repo definition", "namespace",  envConfig.Namespace)
}

func GetEnv(c *gin.Context) {
	envName := c.Param("envName")
	ctrl.Log.Info("Get environment Repo Definition request for", "envName", envName)

}

func ListEnv(c *gin.Context) {
}

func DeleteEnv(c *gin.Context) {

}

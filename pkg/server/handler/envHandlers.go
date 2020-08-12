package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/cloud-native-application/rudrx/pkg/server/apis"
	"github.com/cloud-native-application/rudrx/pkg/server/util"
)

// ENV related handlers
func CreateEnv(c *gin.Context) {
	var envConfig apis.Environment
	if err := c.ShouldBindJSON(&envConfig); err != nil {
		util.HandleError(c, util.InvalidArgument, "the create environment request body is invalid")
		return
	}
	ctrl.Log.Info("Get a create environment request", "env", envConfig)
	// TODO: implement this

	c.Status(http.StatusOK)
}

func GetEnv(c *gin.Context) {
	envName := c.Param("envName")
	ctrl.Log.Info("Get a get environment request", "envName", envName)

	// TODO: implement this
	c.JSON(http.StatusOK, apis.Environment{
		EnvironmentName: envName,
		Namespace:       "test",
	})
}

func ListEnv(c *gin.Context) {
}

func DeleteEnv(c *gin.Context) {

}

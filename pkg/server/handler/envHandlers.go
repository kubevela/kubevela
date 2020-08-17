package handler

import (
	"net/http"

	"github.com/cloud-native-application/rudrx/api/types"

	"github.com/cloud-native-application/rudrx/pkg/oam"

	"github.com/gin-gonic/gin"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/cloud-native-application/rudrx/pkg/server/apis"
	"github.com/cloud-native-application/rudrx/pkg/server/util"
)

// ENV related handlers
func CreateEnv(c *gin.Context) {
	var envConfig types.EnvMeta
	if err := c.ShouldBindJSON(&envConfig); err != nil {
		util.HandleError(c, util.InvalidArgument, "the create environment request body is invalid")
		return
	}
	ctrl.Log.Info("Get a create environment request", "env", envConfig)
	name := envConfig.Name
	namespace := envConfig.Namespace
	if namespace == "" {
		namespace = "default"
	}
	ctx := util.GetContext(c)
	kubeClient := c.MustGet("KubeClient")
	err, message := oam.CreateOrUpdateEnv(ctx, kubeClient.(client.Client), name, namespace)

	var code = http.StatusOK
	if err != nil {
		code = http.StatusInternalServerError
		message = err.Error()
	}
	c.JSON(code, apis.Response{
		Code: code,
		Data: message,
	})
}

func GetEnv(c *gin.Context) {
	envName := c.Param("envName")
	ctrl.Log.Info("Get a get environment request", "envName", envName)
	envList, err := oam.ListEnvs(envName)

	var code = http.StatusOK
	if err != nil {
		code = http.StatusInternalServerError
	}
	c.JSON(code, apis.Response{
		Code: code,
		Data: envList,
	})
}

func ListEnv(c *gin.Context) {
	GetEnv(c)
}

func DeleteEnv(c *gin.Context) {
	envName := c.Param("envName")
	ctrl.Log.Info("Delete a delete environment request", "envName", envName)
	msg, err := oam.DeleteEnv(envName)
	util.AssembleResponse(c, msg, err)
}

func SwitchEnv(c *gin.Context) {
	envName := c.Param("envName")
	ctrl.Log.Info("Patch a switch environment request", "envName", envName)
	msg, err := oam.SwitchEnv(envName)
	util.AssembleResponse(c, msg, err)
}

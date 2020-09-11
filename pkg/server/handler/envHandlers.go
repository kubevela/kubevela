package handler

import (
	"github.com/gin-gonic/gin"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/server/apis"
	"github.com/oam-dev/kubevela/pkg/server/util"
)

// ENV related handlers
func CreateEnv(c *gin.Context) {
	var environment apis.Environment
	if err := c.ShouldBindJSON(&environment); err != nil {
		util.HandleError(c, util.InvalidArgument, "the create environment request body is invalid")
		return
	}
	ctrl.Log.Info("Get a create environment request", "env", environment)
	name := environment.EnvName
	namespace := environment.Namespace
	if namespace == "" {
		namespace = "default"
	}
	ctx := util.GetContext(c)
	kubeClient := c.MustGet("KubeClient")
	message, err := oam.CreateEnv(ctx, kubeClient.(client.Client), name, namespace)
	util.AssembleResponse(c, message, err)
}

func UpdateEnv(c *gin.Context) {
	envName := c.Param("envName")
	ctrl.Log.Info("Put a update environment request", "envName", envName)
	var environmentBody apis.EnvironmentBody
	if err := c.ShouldBindJSON(&environmentBody); err != nil {
		util.HandleError(c, util.InvalidArgument, "the update environment request body is invalid")
		return
	}
	ctx := util.GetContext(c)
	kubeClient := c.MustGet("KubeClient")
	message, err := oam.UpdateEnv(ctx, kubeClient.(client.Client), envName, environmentBody.Namespace)
	util.AssembleResponse(c, message, err)
}

func GetEnv(c *gin.Context) {
	envName := c.Param("envName")
	ctrl.Log.Info("Get a get environment request", "envName", envName)
	envList, err := oam.ListEnvs(envName)

	environmentList := make([]apis.Environment, 0)
	for _, envMeta := range envList {
		environmentList = append(environmentList, apis.Environment{
			EnvName:   envMeta.Name,
			Namespace: envMeta.Namespace,
			Current:   envMeta.Current,
		})
	}
	util.AssembleResponse(c, environmentList, err)
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

func SetEnv(c *gin.Context) {
	envName := c.Param("envName")
	ctrl.Log.Info("Patch a set environment request", "envName", envName)
	msg, err := oam.SetEnv(envName)
	util.AssembleResponse(c, msg, err)
}

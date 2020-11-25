//nolint:golint
package server

import (
	"github.com/gin-gonic/gin"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/server/apis"
	"github.com/oam-dev/kubevela/pkg/server/util"
	"github.com/oam-dev/kubevela/pkg/utils/env"
)

// environment related handlers
func (s *APIServer) CreateEnv(c *gin.Context) {
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
	message, err := env.CreateEnv(ctx, s.KubeClient, name, &types.EnvMeta{
		Name:      name,
		Current:   environment.Current,
		Namespace: namespace,
		Email:     environment.Email,
		Domain:    environment.Domain,
	})
	util.AssembleResponse(c, message, err)
}

func (s *APIServer) UpdateEnv(c *gin.Context) {
	envName := c.Param("envName")
	ctrl.Log.Info("Put a update environment request", "envName", envName)
	var environmentBody apis.EnvironmentBody
	if err := c.ShouldBindJSON(&environmentBody); err != nil {
		util.HandleError(c, util.InvalidArgument, "the update environment request body is invalid")
		return
	}
	ctx := util.GetContext(c)
	message, err := env.UpdateEnv(ctx, s.KubeClient, envName, environmentBody.Namespace)
	util.AssembleResponse(c, message, err)
}

func (s *APIServer) GetEnv(c *gin.Context) {
	envName := c.Param("envName")
	ctrl.Log.Info("Get a get environment request", "envName", envName)
	envList, err := env.ListEnvs(envName)

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

func (s *APIServer) ListEnv(c *gin.Context) {
	s.GetEnv(c)
}

func (s *APIServer) DeleteEnv(c *gin.Context) {
	envName := c.Param("envName")
	ctrl.Log.Info("Delete a delete environment request", "envName", envName)
	msg, err := env.DeleteEnv(envName)
	util.AssembleResponse(c, msg, err)
}

func (s *APIServer) SetEnv(c *gin.Context) {
	envName := c.Param("envName")
	ctrl.Log.Info("Patch a set environment request", "envName", envName)
	msg, err := env.SetEnv(envName)
	util.AssembleResponse(c, msg, err)
}

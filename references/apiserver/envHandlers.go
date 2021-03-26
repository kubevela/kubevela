/*
Copyright 2021 The KubeVela Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package apiserver

import (
	"github.com/gin-gonic/gin"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/utils/env"
	"github.com/oam-dev/kubevela/references/apiserver/apis"
	"github.com/oam-dev/kubevela/references/apiserver/util"
)

// CreateEnv creates an environment
// @Tags environments
// @ID createEnvironment
// @Success 200 {object} apis.Response{code=int,data=string}
// @Failure 500 {object} apis.Response{code=int,data=string}
// @Router /envs/ [post]
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

// UpdateEnv updates an environment
// @Tags environments
// @ID updateEnvironment
// @Param envName path string true "envName"
// @Param body body apis.EnvironmentBody true "envName"
// @Success 200 {object} apis.Response{code=int,data=string}
// @Failure 500 {object} apis.Response{code=int,data=string}
// @Router /envs/{envName} [put]
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

// GetEnv gets an environment
// @Tags environments
// @ID getEnvironment
// @Param envName path string true "envName"
// @Success 200 {object} apis.Response{code=int,data=[]apis.Environment}
// @Failure 500 {object} apis.Response{code=int,data=string}
// @Router /envs/{envName} [get]
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

// ListEnv lists all environments
// @Tags environments
// @ID listEnvironments
// @Accept  json
// @Produce  json
// @success 200 {object} apis.Response{code=int,data=[]apis.Environment}
// @Failure 500 {object} apis.Response{code=int,data=string}
// @Router /envs/ [get]
func (s *APIServer) ListEnv(c *gin.Context) {
	s.GetEnv(c)
}

// DeleteEnv delete an environment
// @Tags environments
// @ID deleteEnvironment
// @Param envName path string true "envName"
// @Success 200 {object} apis.Response{code=int,data=string}
// @Failure 500 {object} apis.Response{code=int,data=string}
// @Router /envs/{envName} [delete]
func (s *APIServer) DeleteEnv(c *gin.Context) {
	envName := c.Param("envName")
	ctrl.Log.Info("Delete a delete environment request", "envName", envName)
	msg, err := env.DeleteEnv(envName)
	util.AssembleResponse(c, msg, err)
}

// SetEnv sets an environment
// @Tags environments
// @ID setEnvironment
// @Param envName path string true "envName"
// @Success 200 {object} apis.Response{code=int,data=string}
// @Failure 500 {object} apis.Response{code=int,data=string}
// @Router /envs/{envName} [patch]
func (s *APIServer) SetEnv(c *gin.Context) {
	envName := c.Param("envName")
	ctrl.Log.Info("Patch a set environment request", "envName", envName)
	msg, err := env.SetEnv(envName)
	util.AssembleResponse(c, msg, err)
}

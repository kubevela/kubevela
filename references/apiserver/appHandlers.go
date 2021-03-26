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
	"fmt"
	"os"

	"github.com/gin-gonic/gin"

	"github.com/oam-dev/kubevela/pkg/utils/env"
	cmdutil "github.com/oam-dev/kubevela/pkg/utils/util"
	"github.com/oam-dev/kubevela/references/apiserver/util"
	"github.com/oam-dev/kubevela/references/appfile/api"
	"github.com/oam-dev/kubevela/references/common"
)

// UpdateApps is placeholder for updating applications
func (s *APIServer) UpdateApps(c *gin.Context) {
}

// GetApp requests an application by the namespaced name in the gin.Context
func (s *APIServer) GetApp(c *gin.Context) {
	envName := c.Param("envName")
	envMeta, err := env.GetEnvByName(envName)
	if err != nil {
		util.HandleError(c, util.StatusInternalServerError, err)
		return
	}
	namespace := envMeta.Namespace
	appName := c.Param("appName")
	ctx := util.GetContext(c)
	applicationMeta, err := common.RetrieveApplicationStatusByName(ctx, s.KubeClient, appName, namespace)
	if err != nil {
		util.HandleError(c, util.StatusInternalServerError, err)
		return
	}
	util.AssembleResponse(c, applicationMeta, nil)
}

// ListApps requests a list of application by the namespace in the gin.Context
// @tags applications
// @ID ListApplications
// @Summary list all applications
// @Param envName path string true "environment name"
// @Success 200 {object} apis.Response{code=int,data=[]apis.ApplicationMeta}
// @Failure 500 {object} apis.Response{code=int,data=string}
// @Router /envs/{envName}/apps [get]
func (s *APIServer) ListApps(c *gin.Context) {
	envName := c.Param("envName")
	envMeta, err := env.GetEnvByName(envName)
	if err != nil {
		util.HandleError(c, util.StatusInternalServerError, err)
		return
	}
	namespace := envMeta.Namespace

	ctx := util.GetContext(c)
	applicationMetaList, err := common.ListApplications(ctx, s.KubeClient, common.Option{Namespace: namespace})
	if err != nil {
		util.HandleError(c, util.StatusInternalServerError, err.Error())
		return
	}
	util.AssembleResponse(c, applicationMetaList, nil)
}

// DeleteApps deletes an application by the namespaced name in the gin.Context
func (s *APIServer) DeleteApps(c *gin.Context) {
	envName := c.Param("envName")
	envMeta, err := env.GetEnvByName(envName)
	if err != nil {
		util.HandleError(c, util.StatusInternalServerError, err)
		return
	}
	appName := c.Param("appName")

	o := common.DeleteOptions{
		Client:  s.KubeClient,
		Env:     envMeta,
		AppName: appName,
	}
	message, err := o.DeleteApp()
	util.AssembleResponse(c, message, err)
}

// CreateApplication creates an application
// @tags applications
// @ID CreateApplication
// @Summary creates an application
// @Param envName path string true "environment name"
// @Param body body appfile.AppFile true "application parameters"
// @Success 200 {object} apis.Response{code=int,data=string}
// @Failure 500 {object} apis.Response{code=int,data=string}
// @Router /envs/{envName}/apps [post]
func (s *APIServer) CreateApplication(c *gin.Context) {
	var body api.AppFile
	if err := c.ShouldBindJSON(&body); err != nil {
		util.HandleError(c, util.InvalidArgument, "the application creation request body is invalid")
		return
	}
	env, err := env.GetEnvByName(c.Param("envName"))
	if err != nil {
		util.HandleError(c, util.StatusInternalServerError, err.Error())
		return
	}
	ioStream := cmdutil.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr}
	o := &common.AppfileOptions{
		Kubecli: s.KubeClient,
		IO:      ioStream,
		Env:     env,
	}
	buildResult, data, err := o.ExportFromAppFile(&body, env.Namespace, false, s.c)
	if err != nil {
		util.HandleError(c, util.StatusInternalServerError, err.Error())
		return
	}
	err = o.BaseAppFileRun(buildResult, data, s.c)
	if err != nil {
		util.HandleError(c, util.StatusInternalServerError, err.Error())
		return
	}
	msg := fmt.Sprintf("application %s is successfully created", body.Name)
	util.AssembleResponse(c, msg, nil)
}

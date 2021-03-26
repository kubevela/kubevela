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
	"os"

	"github.com/gin-gonic/gin"

	"github.com/oam-dev/kubevela/pkg/utils/env"
	cmdutil "github.com/oam-dev/kubevela/pkg/utils/util"
	"github.com/oam-dev/kubevela/references/apiserver/util"
	"github.com/oam-dev/kubevela/references/common"
)

// GetComponent gets a comoponent from cluster
func (s *APIServer) GetComponent(c *gin.Context) {
	envName := c.Param("envName")
	envMeta, err := env.GetEnvByName(envName)
	if err != nil {
		util.HandleError(c, util.StatusInternalServerError, err)
		return
	}
	namespace := envMeta.Namespace
	applicationName := c.Param("appName")
	componentName := c.Param("compName")
	ctx := util.GetContext(c)
	componentMeta, err := common.RetrieveComponent(ctx, s.KubeClient, applicationName, componentName, namespace)
	if err != nil {
		util.HandleError(c, util.StatusInternalServerError, err)
		return
	}
	util.AssembleResponse(c, componentMeta, nil)
}

// DeleteComponent deletes a component from cluster
func (s *APIServer) DeleteComponent(c *gin.Context) {
	envName := c.Param("envName")
	envMeta, err := env.GetEnvByName(envName)
	if err != nil {
		util.HandleError(c, util.StatusInternalServerError, err)
		return
	}
	appName := c.Param("appName")
	componentName := c.Param("compName")

	o := common.DeleteOptions{
		Client:   s.KubeClient,
		Env:      envMeta,
		AppName:  appName,
		CompName: componentName}

	message, err := o.DeleteComponent(
		cmdutil.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr})
	util.AssembleResponse(c, message, err)
}

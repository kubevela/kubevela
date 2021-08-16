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

package services

import (
	"context"
	"net/http"

	"github.com/labstack/echo/v4"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/apis"
)

// ApplicationService serves as Application Open API for request
type ApplicationService struct {
	k8sClient client.Client
}

// NewApplicationService create an application service
func NewApplicationService(kc client.Client) *ApplicationService {
	return &ApplicationService{
		k8sClient: kc,
	}
}

// CreateOrUpdateApplication will create or update application
// POST /v1/namespaces/<namespace>/applications/<appname>
func (s *ApplicationService) CreateOrUpdateApplication(c echo.Context) error {
	namespace := c.Param("namespace")
	name := c.Param("appname")
	appReq := new(apis.ApplicationRequest)
	if err := c.Bind(appReq); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body: " + err.Error()})
	}
	ctx := context.TODO()
	var app, existApp v1beta1.Application
	app.Namespace = namespace
	app.Name = name
	app.Spec.Components = appReq.Components
	app.Spec.Policies = appReq.Policies
	app.Spec.Workflow = appReq.Workflow
	err := s.k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, &existApp)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "fail to get application: " + err.Error()})
		}
		err = s.k8sClient.Create(ctx, &app)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "fail to create application: " + err.Error()})
		}
		return c.JSON(http.StatusOK, struct{}{})
	}
	existApp.Spec = app.Spec
	err = s.k8sClient.Update(ctx, &existApp)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "fail to update application: " + err.Error()})
	}
	return c.JSON(http.StatusOK, struct{}{})
}

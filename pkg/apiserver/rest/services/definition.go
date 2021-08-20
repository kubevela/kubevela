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
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/pkg/apiserver/rest/apis"
	common2 "github.com/oam-dev/kubevela/references/common"
)

// DefinitionService serves as Definition Open API for request
type DefinitionService struct {
	k8sClient client.Client
}

// NewDefinitionService create an application service
func NewDefinitionService(kc client.Client) *DefinitionService {
	return &DefinitionService{
		k8sClient: kc,
	}
}

// GetDefinition will get definition details
// GET /v1/definitions/<name>?type=<xxx>&namespace=<xx>&format=<cue>
func (s *DefinitionService) GetDefinition(c echo.Context) error {
	namespace := c.QueryParam("namespace")
	// format := c.QueryParam("format")  should we add this parameter to support cue format and un-cue(raw definition) format ?
	defType := c.QueryParam("type")
	defName := c.Param("name")

	definitions, err := common2.SearchDefinition(defName, s.k8sClient, defType, namespace)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "fail to get definition: " + err.Error()})
	}
	if len(definitions) == 0 {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "definition does not found: " + err.Error()})
	}

	definition := &common2.Definition{Unstructured: definitions[0]}
	specStr, err := definition.ToCUEString()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "fail to convert definition to CUE string: " + err.Error()})
	}

	var defResp = &apis.DefinitionResponse{
		APIVersion: definition.GetAPIVersion(),
		Kind:       definition.GetKind(),
		Spec:       specStr,
	}

	return c.JSON(http.StatusOK, defResp)
}

// CreateDefinition will create a definition
// POST /v1/definitions?name=<xxx>&namespace=<xx>&format=<xxx>
func (s *DefinitionService) CreateDefinition(c echo.Context) error {
	name := c.Param("defname")
	namespace := c.QueryParam("namespace")
	format := c.QueryParam("format")

	defReq := new(apis.DefinitionRequest)
	if err := c.Bind(defReq); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body: " + err.Error()})
	}
	var def = common2.Definition{
		Unstructured: unstructured.Unstructured{},
	}
	def.SetName(name)
	def.SetNamespace(namespace)

	switch format {
	case "json":
		def.SetAPIVersion(defReq.APIVersion)
		def.SetGVK(defReq.Kind)
		if err := SetUnstructuredSpecFromString(defReq.Spec, &def.Unstructured); err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error ": "failed set spec in unstructured definition"})
		}

	case "cue":
		var config rest.Config // empty configuration
		if err := def.FromCUEString(defReq.CUEString, &config); err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to parse CUE: " + err.Error()})
		}

	default:
		return c.JSON(http.StatusBadRequest, map[string]string{"err": "invalid request format for definition "})
	}

	err := s.k8sClient.Get(context.TODO(), client.ObjectKey{Namespace: namespace, Name: name}, &def)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "fail to get definition: " + err.Error()})
		}

		err = s.k8sClient.Create(context.TODO(), &def)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "fail to create definition: " + err.Error()})
		}
	}

	return c.JSON(http.StatusOK, struct{}{})
}

// SetUnstructuredSpecFromString set UnstructuredObj spec field from string
func SetUnstructuredSpecFromString(spec string, u *unstructured.Unstructured) error {

	data := make(map[string]interface{})
	if err := yaml.Unmarshal([]byte(spec), &data); err != nil {
		return err
	}
	_ = unstructured.SetNestedMap(u.Object, data, "spec")
	return nil
}

func convertUnstructToSpecKind(u *unstructured.Unstructured){

}

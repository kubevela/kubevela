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
	"fmt"
	"net/http"

	echo "github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"

	"github.com/oam-dev/kubevela/pkg/apiserver/common"
	"github.com/oam-dev/kubevela/pkg/apiserver/proto/model"
	"github.com/oam-dev/kubevela/pkg/apiserver/runtime"
)

// SchemaService schema service
type SchemaService struct {
	k8sClient client.Client
}

// NewSchemaService create schema service
func NewSchemaService(client client.Client) *SchemaService {

	return &SchemaService{
		k8sClient: client,
	}
}

// GetWorkloadSchema get workload schema
func (s *SchemaService) GetWorkloadSchema(c echo.Context) error {
	definitionName := c.QueryParam("name")
	definitionNamespace := c.QueryParam("namespace")
	definitionType := c.QueryParam("type")

	clusterName := c.Param("cluster")
	cli, err := s.getClientByClusterName(clusterName)
	if err != nil {
		return err
	}

	key := client.ObjectKey{Namespace: definitionNamespace, Name: definitionName}
	definition, err := GenDefinitionObj(definitionName, definitionType)
	if err != nil {
		return err
	}

	if err := cli.Get(context.Background(), key, definition); err != nil {
		return err
	}

	parse := common.NewParseReference(cli)
	schema, err := parse.ParseDefinition(definition, definitionName, definitionNamespace)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, &model.DefinitionsResponse{
		Definitions: []*model.Definition{schema},
	})
}

// GenDefinitionObj generate definition object
func GenDefinitionObj(name, wType string) (*unstructured.Unstructured, error) {
	obj := &unstructured.Unstructured{}
	obj.SetName(name)
	switch wType {
	case "workload":
		obj.SetGroupVersionKind(v1beta1.WorkloadDefinitionGroupVersionKind)
	case "trait":
		obj.SetGroupVersionKind(v1beta1.TraitDefinitionGroupVersionKind)
	case "component":
		obj.SetGroupVersionKind(v1beta1.ComponentDefinitionGroupVersionKind)
	default:
		return nil, errors.Errorf("not found definition %s", wType)
	}

	return obj, nil
}

func (s *SchemaService) getClientByClusterName(clusterName string) (client.Client, error) {
	var cm v1.ConfigMap
	// k8sClient is a common client for getting configmap info in current cluster.
	err := s.k8sClient.Get(context.Background(), client.ObjectKey{Namespace: DefaultUINamespace, Name: clusterName}, &cm) // cluster configmap info
	if err != nil {
		return nil, fmt.Errorf("unable to find configmap parameters in %s:%w ", clusterName, err)
	}

	// cli is the client running in specific cluster to get specific k8s cr resource.
	cli, err := runtime.GetClient([]byte(cm.Data["Kubeconfig"]))
	if err != nil {
		return nil, err
	}
	return cli, nil
}

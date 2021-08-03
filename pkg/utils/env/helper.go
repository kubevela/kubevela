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

package env

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	common2 "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/pkg/utils/common"
)

// getEnvAppList will filter application that represent an env
func getEnvAppList() ([]v1beta1.Application, error) {
	list := v1beta1.ApplicationList{}
	clt, err := common.GetClient()
	if err != nil {
		return nil, err
	}
	ctx := context.Background()
	matchLabels := metav1.LabelSelector{
		MatchExpressions: []metav1.LabelSelectorRequirement{{
			Key:      IndicatingLabel,
			Operator: metav1.LabelSelectorOpExists,
		}},
	}
	selector, err := metav1.LabelSelectorAsSelector(&matchLabels)
	if err != nil {
		return nil, err
	}
	err = clt.List(ctx, &list, &client.ListOptions{LabelSelector: selector})

	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

func getEnvNamespace(application *v1beta1.Application) string {
	namespace := DefaultEnvNamespace
	for _, comp := range application.Spec.Components {
		if comp.Type == RawType {
			obj, err := util.RawExtension2Unstructured(&comp.Properties)
			if err != nil {
				return ""
			}
			return obj.GetName()
		}
	}
	return namespace
}

// add namespace object if env namespace is not default
func addNamespaceObjectIfNeeded(meta *types.EnvMeta, app *v1beta1.Application) {
	if meta.Namespace != DefaultEnvNamespace {
		app.Spec.Components = append(app.Spec.Components, common2.ApplicationComponent{
			Name: meta.Namespace,
			Type: RawType,
			Properties: util.Object2RawExtension(map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Namespace",
				"metadata": map[string]string{
					"name": meta.Namespace,
				},
			}),
		})
	}
}

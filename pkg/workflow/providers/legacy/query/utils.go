/*
 Copyright 2022. The KubeVela Authors.

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

package query

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/oam-dev/kubevela/pkg/oam"
	querytypes "github.com/oam-dev/kubevela/pkg/utils/types"
)

func buildResourceArray(res querytypes.AppliedResource, parent, node *querytypes.ResourceTreeNode, kind string, apiVersion string) (pods []querytypes.ResourceItem) {
	if node.LeafNodes != nil {
		for _, subNode := range node.LeafNodes {
			pods = append(pods, buildResourceArray(res, node, subNode, kind, apiVersion)...)
		}
	} else if node.Kind == kind && node.APIVersion == apiVersion {
		pods = append(pods, buildResourceItem(res, querytypes.Workload{
			APIVersion: parent.APIVersion,
			Kind:       parent.Kind,
			Name:       parent.Name,
			Namespace:  parent.Namespace,
		}, node.Object))
	}
	return
}

func buildResourceItem(res querytypes.AppliedResource, workload querytypes.Workload, object *unstructured.Unstructured) querytypes.ResourceItem {
	return querytypes.ResourceItem{
		Cluster:   res.Cluster,
		Workload:  workload,
		Component: res.Component,
		Object:    object,
		PublishVersion: func() string {
			if object.GetAnnotations()[oam.AnnotationPublishVersion] != "" {
				return object.GetAnnotations()[oam.AnnotationPublishVersion]
			}
			return res.PublishVersion
		}(),
		DeployVersion: func() string {
			if object.GetAnnotations()[oam.AnnotationDeployVersion] != "" {
				return object.GetAnnotations()[oam.AnnotationDeployVersion]
			}
			return res.DeployVersion
		}(),
	}
}

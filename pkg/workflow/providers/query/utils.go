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
		}, node.Object, node.AdditionalInfo))
	}
	return
}

func buildResourceItem(res querytypes.AppliedResource, workload querytypes.Workload, object *unstructured.Unstructured, addInfo map[string]interface{}) querytypes.ResourceItem {
	object = withoutSecretData(object)
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
		AdditionalInfo: addInfo,
	}
}

// withoutSecretData is object without the values of a Secret: its data, its stringData and the
// last-applied annotations, kubectl's and KubeVela's, that repeat them. A query runs with the controller's rights, and a
// resource tree reaches Secrets its caller may not read, so a Secret is reported by name and
// metadata only. Any other object is returned as it is.
func withoutSecretData(object *unstructured.Unstructured) *unstructured.Unstructured {
	if object == nil || object.GetKind() != "Secret" || object.GetAPIVersion() != "v1" {
		return object
	}
	stripped := object.DeepCopy()
	unstructured.RemoveNestedField(stripped.Object, "data")
	unstructured.RemoveNestedField(stripped.Object, "stringData")
	if annotations := stripped.GetAnnotations(); annotations != nil {
		delete(annotations, "kubectl.kubernetes.io/last-applied-configuration")
		delete(annotations, oam.AnnotationLastAppliedConfig)
		stripped.SetAnnotations(annotations)
	}
	return stripped
}

// withoutSecretDataIn redacts each listed resource's object as withoutSecretData does.
func withoutSecretDataIn(resources []Resource) []Resource {
	for i := range resources {
		resources[i].Object = withoutSecretData(resources[i].Object)
	}
	return resources
}

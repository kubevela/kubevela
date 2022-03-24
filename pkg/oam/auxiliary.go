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

package oam

import (
	"github.com/crossplane/crossplane-runtime/pkg/meta"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// SetCluster add cluster label to object
func SetCluster(o client.Object, clusterName string) {
	meta.AddLabels(o, map[string]string{LabelAppCluster: clusterName})
}

// GetCluster get cluster from object
func GetCluster(o client.Object) string {
	if labels := o.GetLabels(); labels != nil {
		return labels[LabelAppCluster]
	}
	return ""
}

// GetServiceAccountNameFromAnnotations extracts the service account name from the given object's annotations.
func GetServiceAccountNameFromAnnotations(o client.Object) string {
	if annotations := o.GetAnnotations(); annotations != nil {
		return annotations[AnnotationServiceAccountName]
	}
	return ""
}

// GetPublishVersion get PublishVersion from object
func GetPublishVersion(o client.Object) string {
	if annotations := o.GetAnnotations(); annotations != nil {
		return annotations[AnnotationPublishVersion]
	}
	return ""
}

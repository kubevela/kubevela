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
	"time"

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

// GetPublishVersion get PublishVersion from object
func GetPublishVersion(o client.Object) string {
	if annotations := o.GetAnnotations(); annotations != nil {
		return annotations[AnnotationPublishVersion]
	}
	return ""
}

// GetDeployVersion get DeployVersion from object
func GetDeployVersion(o client.Object) string {
	if annotations := o.GetAnnotations(); annotations != nil {
		return annotations[AnnotationDeployVersion]
	}
	return ""
}

// GetLastAppliedTime .
func GetLastAppliedTime(o client.Object) time.Time {
	if annotations := o.GetAnnotations(); annotations != nil {
		s := annotations[AnnotationLastAppliedTime]
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			return t
		}
	}
	return o.GetCreationTimestamp().Time
}

// SetPublishVersion set PublishVersion for object
func SetPublishVersion(o client.Object, publishVersion string) {
	annotations := o.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}
	annotations[AnnotationPublishVersion] = publishVersion
	o.SetAnnotations(annotations)
}

// GetControllerRequirement get ControllerRequirement from object
func GetControllerRequirement(o client.Object) string {
	if annotations := o.GetAnnotations(); annotations != nil {
		return annotations[AnnotationControllerRequirement]
	}
	return ""
}

// SetControllerRequirement set ControllerRequirement for object
func SetControllerRequirement(o client.Object, controllerRequirement string) {
	annotations := o.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}
	annotations[AnnotationControllerRequirement] = controllerRequirement
	if controllerRequirement == "" {
		delete(annotations, AnnotationControllerRequirement)
	}
	o.SetAnnotations(annotations)
}

/*
 Copyright 2021. The KubeVela Authors.

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

package v1alpha1

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8sscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/scheme"

	workflowv1alpha1 "github.com/kubevela/workflow/api/v1alpha1"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
)

// Package type metadata.
const (
	Group   = common.Group
	Version = "v1alpha1"
)

var (
	// SchemeGroupVersion is group version used to register these objects
	SchemeGroupVersion = schema.GroupVersion{Group: Group, Version: Version}

	// SchemeBuilder is used to add go types to the GroupVersionKind scheme
	SchemeBuilder = &scheme.Builder{GroupVersion: SchemeGroupVersion}

	// AddToScheme is a global function that registers this API group & version to a scheme
	AddToScheme = SchemeBuilder.AddToScheme
)

// Policy meta
var (
	PolicyKind             = "Policy"
	PolicyGroupVersionKind = SchemeGroupVersion.WithKind(PolicyKind)
)

// Workflow meta
var (
	WorkflowKind             = "Workflow"
	WorkflowGroupVersionKind = SchemeGroupVersion.WithKind(WorkflowKind)
)

func init() {
	SchemeBuilder.Register(&Policy{}, &PolicyList{})
	SchemeBuilder.Register(&workflowv1alpha1.Workflow{}, &workflowv1alpha1.WorkflowList{})
	_ = SchemeBuilder.AddToScheme(k8sscheme.Scheme)
}

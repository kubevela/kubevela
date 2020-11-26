/*
Copyright 2020 The Crossplane Authors.

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

package applicationconfiguration

import (
	corev1 "k8s.io/api/core/v1"

	"github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
)

// dag is the dependency graph for an AppConfig.
type dag struct {
	Sources map[string]*dagSource
}

// dagSource represents the object information with DataOutput
type dagSource struct {
	// ObjectRef refers to the object this source come from.
	ObjectRef *corev1.ObjectReference

	Conditions []v1alpha2.ConditionRequirement
}

// newDAG creates a fresh new DAG.
func newDAG() *dag {
	return &dag{
		Sources: make(map[string]*dagSource),
	}
}

// AddSource adds a data output source into the DAG.
func (d *dag) AddSource(sourceName string, ref *corev1.ObjectReference, m []v1alpha2.ConditionRequirement) {
	d.Sources[sourceName] = &dagSource{
		ObjectRef:  ref,
		Conditions: m,
	}
}

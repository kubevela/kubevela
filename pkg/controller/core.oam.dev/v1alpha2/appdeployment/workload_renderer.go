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

package appdeployment

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	oamcorealpha "github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	pkgutil "github.com/oam-dev/kubevela/pkg/oam/util"
)

var (
	errCompNotFound = errors.New("Component not found")
)

// WorkloadRenderer renders Workloads which are k8s resources to apply
type WorkloadRenderer interface {
	Render(ctx context.Context, ac *oamcorealpha.ApplicationConfiguration, comps []*oamcorealpha.Component) ([]*workload, error)
}

type workloadRenderer struct {
	kubecli client.Client
}

// NewWorkloadRenderer is the main entry to create WorkloadRenderer implementation.
func NewWorkloadRenderer(cli client.Client) WorkloadRenderer {
	return &workloadRenderer{kubecli: cli}
}
func (r *workloadRenderer) Render(ctx context.Context,
	ac *oamcorealpha.ApplicationConfiguration,
	comps []*oamcorealpha.Component) ([]*workload, error) {

	var workloads []*workload

	for _, acc := range ac.Spec.Components {
		comp := findComponent(acc.ComponentName, comps)
		if comp == nil {
			return nil, errors.Wrap(errCompNotFound, fmt.Sprintf("try to find %s", acc.ComponentName))
		}

		obj, err := pkgutil.Object2Map(comp.Spec.Workload)
		if err != nil {
			return nil, err
		}
		traits, err := convertWorkloadTraits(comp.Name, comp.Namespace, acc.Traits)
		if err != nil {
			return nil, err
		}
		workloads = append(workloads, newWorkload(comp.Name, comp.Namespace, obj, traits))

	}

	return workloads, nil
}

func convertWorkloadTraits(name, ns string, acTraits []oamcorealpha.ComponentTrait) ([]*workloadTrait, error) {
	var traits []*workloadTrait
	for _, acTrait := range acTraits {
		obj, err := pkgutil.Object2Map(acTrait.Trait)
		if err != nil {
			return nil, err
		}
		traits = append(traits, newWorkloadTrait(name, ns, obj))
	}
	return traits, nil
}

func findComponent(name string, comps []*oamcorealpha.Component) *oamcorealpha.Component {
	for _, comp := range comps {
		if comp.Name == name {
			return comp
		}
	}
	return nil
}

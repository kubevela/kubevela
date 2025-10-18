/*
Copyright 2025 The KubeVela Authors.

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

package config

import (
	"github.com/spf13/pflag"

	oamcontroller "github.com/oam-dev/kubevela/pkg/controller/core.oam.dev"
)

// ControllerConfig wraps the oamcontroller.Args configuration.
// While this appears to duplicate the Args struct, it serves as the new home for
// controller flag registration after the AddFlags method was moved here from
// the oamcontroller package during refactoring.
type ControllerConfig struct {
	// Embed the existing Args struct to reuse its fields
	oamcontroller.Args
}

// NewControllerConfig creates a new ControllerConfig with defaults.
func NewControllerConfig() *ControllerConfig {
	return &ControllerConfig{
		Args: oamcontroller.Args{
			RevisionLimit:                                50,
			AppRevisionLimit:                             10,
			DefRevisionLimit:                             20,
			AutoGenWorkloadDefinition:                    true,
			ConcurrentReconciles:                         4,
			IgnoreAppWithoutControllerRequirement:        false,
			IgnoreDefinitionWithoutControllerRequirement: false,
		},
	}
}

// AddFlags registers controller configuration flags.
// This method was moved here from oamcontroller.Args during refactoring
// to centralize configuration management.
func (c *ControllerConfig) AddFlags(fs *pflag.FlagSet) {
	fs.IntVar(&c.RevisionLimit, "revision-limit", c.RevisionLimit,
		"RevisionLimit is the maximum number of revisions that will be maintained. The default value is 50.")
	fs.IntVar(&c.AppRevisionLimit, "application-revision-limit", c.AppRevisionLimit,
		"application-revision-limit is the maximum number of application useless revisions that will be maintained, if the useless revisions exceed this number, older ones will be GCed first.The default value is 10.")
	fs.IntVar(&c.DefRevisionLimit, "definition-revision-limit", c.DefRevisionLimit,
		"definition-revision-limit is the maximum number of component/trait definition useless revisions that will be maintained, if the useless revisions exceed this number, older ones will be GCed first.The default value is 20.")
	fs.BoolVar(&c.AutoGenWorkloadDefinition, "autogen-workload-definition", c.AutoGenWorkloadDefinition,
		"Automatic generated workloadDefinition which componentDefinition refers to.")
	fs.IntVar(&c.ConcurrentReconciles, "concurrent-reconciles", c.ConcurrentReconciles,
		"concurrent-reconciles is the concurrent reconcile number of the controller. The default value is 4")
	fs.BoolVar(&c.IgnoreAppWithoutControllerRequirement, "ignore-app-without-controller-version", c.IgnoreAppWithoutControllerRequirement,
		"If true, application controller will not process the app without 'app.oam.dev/controller-version-require' annotation")
	fs.BoolVar(&c.IgnoreDefinitionWithoutControllerRequirement, "ignore-definition-without-controller-version", c.IgnoreDefinitionWithoutControllerRequirement,
		"If true, trait/component/workflowstep definition controller will not process the definition without 'definition.oam.dev/controller-version-require' annotation")
}

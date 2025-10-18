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

// ControllerConfig contains controller-level configuration.
type ControllerConfig struct {
	RevisionLimit                                int
	AppRevisionLimit                             int
	DefRevisionLimit                             int
	AutoGenWorkloadDefinition                    bool
	ConcurrentReconciles                         int
	IgnoreAppWithoutControllerRequirement        bool
	IgnoreDefinitionWithoutControllerRequirement bool
}

// NewControllerConfig creates a new ControllerConfig with defaults.
func NewControllerConfig() *ControllerConfig {
	return &ControllerConfig{
		RevisionLimit:                                50,
		AppRevisionLimit:                             10,
		DefRevisionLimit:                             20,
		AutoGenWorkloadDefinition:                    true,
		ConcurrentReconciles:                         4,
		IgnoreAppWithoutControllerRequirement:        false,
		IgnoreDefinitionWithoutControllerRequirement: false,
	}
}

// AddFlags registers controller configuration flags.
func (c *ControllerConfig) AddFlags(fs *pflag.FlagSet) {
	fs.IntVar(&c.RevisionLimit, "revision-limit", c.RevisionLimit,
		"revision-limit is the maximum number of revisions that will be maintained. The default value is 50.")
	fs.IntVar(&c.AppRevisionLimit, "application-revision-limit", c.AppRevisionLimit,
		"application-revision-limit is the maximum number of application useless revisions that will be maintained, if the useless revisions exceed this number, older ones will be GCed first.The default value is 10.")
	fs.IntVar(&c.DefRevisionLimit, "definition-revision-limit", c.DefRevisionLimit,
		"definition-revision-limit is the maximum number of component/trait definition useless revisions that will be maintained, if the useless revisions exceed this number, older ones will be GCed first.The default value is 20.")
	fs.BoolVar(&c.AutoGenWorkloadDefinition, "autogen-workload-definition", c.AutoGenWorkloadDefinition,
		"Automatic generated workloadDefinition which componentDefinition refers to.")
	fs.IntVar(&c.ConcurrentReconciles, "concurrent-reconciles", c.ConcurrentReconciles,
		"concurrent-reconciles is the concurrent reconcile number of the controller. The default value is 4")
	fs.BoolVar(&c.IgnoreAppWithoutControllerRequirement, "ignore-app-no-controller-req", c.IgnoreAppWithoutControllerRequirement,
		"If true, application controller will not process the application without 'app.oam.dev/controller-version-require' annotation")
	fs.BoolVar(&c.IgnoreDefinitionWithoutControllerRequirement, "ignore-def-no-controller-req", c.IgnoreDefinitionWithoutControllerRequirement,
		"If true, definition controller will not process the definition without 'definition.oam.dev/controller-version-require' annotation")
}

// ToArgs converts ControllerConfig to oamcontroller.Args for backward compatibility with existing controller setup code.
func (c *ControllerConfig) ToArgs() oamcontroller.Args {
	return oamcontroller.Args{
		RevisionLimit:                                c.RevisionLimit,
		AppRevisionLimit:                             c.AppRevisionLimit,
		DefRevisionLimit:                             c.DefRevisionLimit,
		AutoGenWorkloadDefinition:                    c.AutoGenWorkloadDefinition,
		ConcurrentReconciles:                         c.ConcurrentReconciles,
		IgnoreAppWithoutControllerRequirement:        c.IgnoreAppWithoutControllerRequirement,
		IgnoreDefinitionWithoutControllerRequirement: c.IgnoreDefinitionWithoutControllerRequirement,
	}
}

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

package core_oam_dev

import (
	"time"

	"github.com/spf13/pflag"

	"github.com/kubevela/workflow/pkg/cue/packages"

	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
)

// ApplyOnceOnlyMode enumerates ApplyOnceOnly modes.
type ApplyOnceOnlyMode string

const (
	// ApplyOnceOnlyOff indicates workloads and traits should always be affected.
	// It means ApplyOnceOnly is disabled.
	ApplyOnceOnlyOff ApplyOnceOnlyMode = "off"

	// ApplyOnceOnlyOn indicates workloads and traits should not be affected
	// if no spec change is made in the ApplicationConfiguration.
	ApplyOnceOnlyOn ApplyOnceOnlyMode = "on"

	// ApplyOnceOnlyForce is a more strong case for ApplyOnceOnly, the workload
	// and traits won't be affected if no spec change is made in the ApplicationConfiguration,
	// even if the workload or trait has been deleted from cluster.
	ApplyOnceOnlyForce ApplyOnceOnlyMode = "force"
)

// Args args used by controller
type Args struct {

	// RevisionLimit is the maximum number of revisions that will be maintained.
	// The default value is 50.
	RevisionLimit int

	// AppRevisionLimit is the maximum number of application revisions that will be maintained.
	// The default value is 10.
	AppRevisionLimit int

	// DefRevisionLimit is the maximum number of component/trait definition revisions that will be maintained.
	// The default value is 20.
	DefRevisionLimit int

	// ApplyMode indicates whether workloads and traits should be
	// affected if no spec change is made in the ApplicationConfiguration.
	ApplyMode ApplyOnceOnlyMode

	// CustomRevisionHookURL is a webhook which will let oam-runtime to call with AC+Component info
	// The webhook server will return a customized component revision for oam-runtime
	CustomRevisionHookURL string

	// DiscoveryMapper used for CRD discovery in controller, a K8s client is contained in it.
	DiscoveryMapper discoverymapper.DiscoveryMapper
	// PackageDiscover used for CRD discovery in CUE packages, a K8s client is contained in it.
	PackageDiscover *packages.PackageDiscover

	// ConcurrentReconciles is the concurrent reconcile number of the controller
	ConcurrentReconciles int

	// DependCheckWait is the time to wait for ApplicationConfiguration's dependent-resource ready
	DependCheckWait time.Duration

	// AutoGenWorkloadDefinition indicates whether automatic generated workloadDefinition which componentDefinition refers to
	AutoGenWorkloadDefinition bool

	// OAMSpecVer is the oam spec version controller want to setup
	OAMSpecVer string

	// EnableCompatibility indicates that will change some functions of controller to adapt to multiple platforms, such as asi.
	EnableCompatibility bool

	// IgnoreAppWithoutControllerRequirement indicates that application controller will not process the app without 'app.oam.dev/controller-version-require' annotation.
	IgnoreAppWithoutControllerRequirement bool

	// IgnoreDefinitionWithoutControllerRequirement indicates that trait/component/workflowstep definition controller will not process the definition without 'definition.oam.dev/controller-version-require' annotation.
	IgnoreDefinitionWithoutControllerRequirement bool
}

// AddFlags adds flags to the specified FlagSet
func (a *Args) AddFlags(fs *pflag.FlagSet, c *Args) {
	fs.IntVar(&a.RevisionLimit, "revision-limit", c.RevisionLimit,
		"RevisionLimit is the maximum number of revisions that will be maintained. The default value is 50.")
	fs.IntVar(&a.AppRevisionLimit, "application-revision-limit", c.AppRevisionLimit,
		"application-revision-limit is the maximum number of application useless revisions that will be maintained, if the useless revisions exceed this number, older ones will be GCed first.The default value is 10.")
	fs.IntVar(&a.DefRevisionLimit, "definition-revision-limit", c.DefRevisionLimit,
		"definition-revision-limit is the maximum number of component/trait definition useless revisions that will be maintained, if the useless revisions exceed this number, older ones will be GCed first.The default value is 20.")
	fs.StringVar(&a.CustomRevisionHookURL, "custom-revision-hook-url", c.CustomRevisionHookURL,
		"custom-revision-hook-url is a webhook url which will let KubeVela core to call with applicationConfiguration and component info and return a customized component revision")
	fs.BoolVar(&a.AutoGenWorkloadDefinition, "autogen-workload-definition", c.AutoGenWorkloadDefinition, "Automatic generated workloadDefinition which componentDefinition refers to.")
	fs.IntVar(&a.ConcurrentReconciles, "concurrent-reconciles", c.ConcurrentReconciles, "concurrent-reconciles is the concurrent reconcile number of the controller. The default value is 4")
	fs.DurationVar(&a.DependCheckWait, "depend-check-wait", c.DependCheckWait, "depend-check-wait is the time to wait for ApplicationConfiguration's dependent-resource ready."+
		"The default value is 30s, which means if dependent resources were not prepared, the ApplicationConfiguration would be reconciled after 30s.")
	fs.StringVar(&a.OAMSpecVer, "oam-spec-ver", c.OAMSpecVer, "oam-spec-ver is the oam spec version controller want to setup, available options: v0.2, v0.3, all")
	fs.BoolVar(&a.EnableCompatibility, "enable-asi-compatibility", c.EnableCompatibility, "enable compatibility for asi")
	fs.BoolVar(&a.IgnoreAppWithoutControllerRequirement, "ignore-app-without-controller-version", c.IgnoreAppWithoutControllerRequirement, "If true, application controller will not process the app without 'app.oam.dev/controller-version-require' annotation")
	fs.BoolVar(&a.IgnoreDefinitionWithoutControllerRequirement, "ignore-definition-without-controller-version", c.IgnoreDefinitionWithoutControllerRequirement, "If true, trait/component/workflowstep definition controller will not process the definition without 'definition.oam.dev/controller-version-require' annotation")
}

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
	"github.com/oam-dev/kubevela/pkg/dsl/definition"
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
	// ApplicationConfigurationInstalled indicates if we have installed the ApplicationConfiguration CRD
	ApplicationConfigurationInstalled bool

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
	PackageDiscover *definition.PackageDiscover
}

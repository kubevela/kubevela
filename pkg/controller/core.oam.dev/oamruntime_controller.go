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

	// ConcurrentReconciles is the concurrent reconcile number of the controller
	ConcurrentReconciles int

	// AutoGenWorkloadDefinition indicates whether automatic generated workloadDefinition which componentDefinition refers to
	AutoGenWorkloadDefinition bool

	// IgnoreAppWithoutControllerRequirement indicates that application controller will not process the app without 'app.oam.dev/controller-version-require' annotation.
	IgnoreAppWithoutControllerRequirement bool

	// IgnoreDefinitionWithoutControllerRequirement indicates that trait/component/workflowstep definition controller will not process the definition without 'definition.oam.dev/controller-version-require' annotation.
	IgnoreDefinitionWithoutControllerRequirement bool
}

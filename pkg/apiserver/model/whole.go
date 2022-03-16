/*
 Copyright 2022 The KubeVela Authors.

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

package model

const (
	// AutoGenDesc describes the metadata in datastore that's automatically generated
	AutoGenDesc = "Automatically converted from KubeVela Application in Kubernetes."

	// AutoGenProj describes the automatically created project
	AutoGenProj = "Automatically generated by sync mechanism."

	// AutoGenEnvNamePrefix describes the common prefix for auto-generated env
	AutoGenEnvNamePrefix = "auto-sync-"
	// AutoGenComp describes the creator of component that is auto-generated
	AutoGenComp = "auto-sync-comp"
	// AutoGenPolicy describes the creator of policy that is auto-generated
	AutoGenPolicy = "auto-sync-policy"
	// AutoGenRefPolicy describes the creator of policy that is auto-generated, this differs from AutoGenPolicy as the policy is referenced ones
	AutoGenRefPolicy = "auto-sync-ref-policy"
	// AutoGenWorkflowNamePrefix describes the common prefix for auto-generated workflow
	AutoGenWorkflowNamePrefix = "auto-sync-"
	// AutoGenTargetNamePrefix describes the common prefix for auto-generated target
	AutoGenTargetNamePrefix = "auto-sync-"

	// LabelSyncGeneration describes the generation synced from
	LabelSyncGeneration = "ux.oam.dev/synced-generation"
	// LabelSyncNamespace describes the namespace synced from
	LabelSyncNamespace = "ux.oam.dev/from-namespace"
	// LabelAppMetaFormatVersion describes the format version of app meta
	LabelAppMetaFormatVersion = "ux.oam.dev/app-meta-format"
)

// DataStoreApp is a memory struct that describes the model of an application in datastore
type DataStoreApp struct {
	AppMeta  *Application
	Env      *Env
	Eb       *EnvBinding
	Comps    []*ApplicationComponent
	Policies []*ApplicationPolicy
	Workflow *Workflow
	Targets  []*Target
}

const (

	// DefaultInitName is default object name for initialization
	DefaultInitName = "default"

	// DefaultAddonProject is default addon projects
	DefaultAddonProject = "addons"

	// DefaultInitNamespace is default namespace name for initialization
	DefaultInitNamespace = "default"

	// DefaultTargetDescription describes default target created
	DefaultTargetDescription = "Default target is created by velaux system automatically."
	// DefaultEnvDescription describes default env created
	DefaultEnvDescription = "Default environment is created by velaux system automatically."
	// DefaultProjectDescription describes the default project created
	DefaultProjectDescription = "Default project is created by velaux system automatically."
)

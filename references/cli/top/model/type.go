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

import "github.com/rivo/tview"

type (
	// View is an abstract of view of app
	View interface {
		Primitive
		Initer
		Hinter
	}
	// Primitive is an abstract of tview ui component
	Primitive interface {
		tview.Primitive
		// Name return name of the component
		Name() string
	}

	// Initer is an abstract of components whose need to init
	Initer interface {
		// Start the component
		Start()
		// Stop the component
		Stop()
		// Init the component
		Init()
	}

	// Hinter is an abstract of components which can provide menu hints to menu component
	Hinter interface {
		// Hint return key action menu hints of the component
		Hint() []MenuHint
	}
)

var (
	// CtxKeyAppName request context key of application name
	CtxKeyAppName = "appName"
	// CtxKeyNamespace request context key of namespace name
	CtxKeyNamespace = "appNs"
	// CtxKeyCluster request context key of cluster name
	CtxKeyCluster = "cluster"
	// CtxKeyClusterNamespace request context key of cluster namespace name
	CtxKeyClusterNamespace = "cluster"
	// CtxKeyComponentName request context key of component name
	CtxKeyComponentName = "componentName"
	// CtxKeyGVR request context key of GVR
	CtxKeyGVR = "gvr"
	// CtxKeyPod request context key of pod
	CtxKeyPod = "pod"
	// CtxKeyContainer request context key of container
	CtxKeyContainer = "container"
)

const (
	// AllNamespace represent all namespaces
	AllNamespace = "all"
	// AllClusterNamespace represent all cluster namespace
	AllClusterNamespace = "all"
	// AllCluster represent all cluster
	AllCluster = "all"
)

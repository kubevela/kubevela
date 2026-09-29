/*
Copyright 2026 The KubeVela Authors.

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

package propexpr

// The placement calls a component read may make.
const (
	// PlaceCluster names a cluster: component.db.cluster("data").
	PlaceCluster = "cluster"
	// PlaceNamespace names a namespace, in the reader's cluster unless a cluster
	// call precedes it: component.db.namespace("orders").
	PlaceNamespace = "namespace"
)

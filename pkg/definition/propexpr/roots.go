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
// Package propexpr declares what a KubeVela property expression may read: the
// source, context and component roots, and which render surface offers which
// context fields.
//
//	properties:
//	  cluster: '$(source.clusterInfo.region + "-cluster")'
//	  owner:   '$("owner" in context.appLabels ? context.appLabels["owner"] : "unassigned")'
package propexpr

import "github.com/kubevela/pkg/cel/template"

// SourceIdent reads a resolved source. It is one of a fixed set of roots, with
// ContextIdent and ComponentIdent: everything a consumer may read hangs off one
// of them, so the sandbox is "these names and nothing else" rather than a
// denylist.
const SourceIdent = "source"

// ComponentIdent reads another component's live output once it is healthy:
// component.<name>.output for the workload and
// component.<name>.outputs.<resource> for a trait resource, beside the reader
// or at a placement named with cluster, namespace or placements. Only component
// and trait properties offer it, because only a component's render can wait: a
// reader whose producer is not ready is simply not healthy yet.
const ComponentIdent = "component"

// IsSource reports whether the reference reads a resolved source.
func IsSource(r template.Reference) bool { return r.Root == SourceIdent }

// IsComponent reports whether the reference reads another component's output.
func IsComponent(r template.Reference) bool { return r.Root == ComponentIdent }

// Placement reports the placement calls a component read makes straight after
// the component, in order, and the path without them. Calls anywhere else stay
// in the path, where validation finds them.
func Placement(r template.Reference) ([]string, []string) {
	if !IsComponent(r) || len(r.Path) < 2 {
		return nil, r.Path
	}
	var calls []string
	i := 1
	for ; i < len(r.Path); i++ {
		call, ok := template.SegmentQualifier(r.Path[i])
		if !ok {
			break
		}
		calls = append(calls, call)
	}
	return calls, append([]string{r.Path[0]}, r.Path[i:]...)
}

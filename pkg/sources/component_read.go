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

package sources

import (
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"

	"k8s.io/apimachinery/pkg/runtime"

	celengine "github.com/kubevela/pkg/cel"
	"github.com/kubevela/pkg/cel/template"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/pkg/definition/celexpr"
	"github.com/oam-dev/kubevela/pkg/definition/propexpr"
)

// ComponentRead is one `component.<name>...` read in a component's properties or
// in one of its traits' properties.
type ComponentRead struct {
	// Trait is the index of the trait whose properties hold the read, or -1 for
	// the component's own properties.
	Trait int
	// Producer is the component being read.
	Producer string
	// Placement is the placement calls the read makes, in order, as
	// template.Call spells them; empty for the producer beside the reader.
	Placement []string
	// Path is the read below the producer and its placement: ["output", ...] or
	// ["outputs", <resource>, ...].
	Path []string
	// Guarded records that the read sits under has() or a ternary arm testing it.
	Guarded bool
}

// String renders the read the way the author wrote it.
func (r ComponentRead) String() string {
	path := []string{r.Producer}
	for _, call := range r.Placement {
		path = append(path, template.QualifierSegment(call))
	}
	return template.Reference{Root: propexpr.ComponentIdent, Path: append(path, r.Path...)}.String()
}

// ReadTarget is where a read's placement calls point.
type ReadTarget struct {
	// Cluster and Namespace name one placement; either may be empty, meaning the
	// reader's own cluster, or the one namespace the producer has in Cluster.
	Cluster, Namespace string
}

// Target reads the placement calls: none, cluster(), namespace() or
// cluster().namespace(), and nothing else.
func (r ComponentRead) Target() (ReadTarget, error) {
	var t ReadTarget
	fns := make([]string, 0, len(r.Placement))
	for _, call := range r.Placement {
		fn, arg := template.SplitCall(call)
		fns = append(fns, fn)
		switch fn {
		case propexpr.PlaceCluster:
			t.Cluster = arg
		case propexpr.PlaceNamespace:
			t.Namespace = arg
		}
	}
	switch strings.Join(fns, ".") {
	case "", propexpr.PlaceCluster, propexpr.PlaceNamespace,
		propexpr.PlaceCluster + "." + propexpr.PlaceNamespace:
		return t, nil
	}
	return ReadTarget{}, fmt.Errorf("%s: name a placement with .cluster(\"<cluster>\"), .namespace(\"<namespace>\") "+
		"or .cluster(\"<cluster>\").namespace(\"<namespace>\")", r)
}

// Misplaced reports a placement call anywhere but straight after the component.
func (r ComponentRead) Misplaced() bool {
	for _, seg := range r.Path {
		if _, ok := template.SegmentQualifier(seg); ok {
			return true
		}
	}
	return false
}

// ComponentReads returns every read of another component's output that a
// component makes, in its own properties and then in each trait's, sorted
// within each.
func ComponentReads(comp common.ApplicationComponent) ([]ComponentRead, error) {
	out, err := readsIn(comp.Properties, -1)
	if err != nil {
		return nil, err
	}
	for i, tr := range comp.Traits {
		reads, err := readsIn(tr.Properties, i)
		if err != nil {
			return nil, err
		}
		out = append(out, reads...)
	}
	return out, nil
}

func readsIn(raw *runtime.RawExtension, trait int) ([]ComponentRead, error) {
	if raw == nil || len(raw.Raw) == 0 {
		return nil, nil
	}
	plan, err := celexpr.Vela.PlanJSON(raw.Raw)
	var faults celengine.CheckErrors
	switch {
	case errors.As(err, &faults):
		return nil, err
	case err != nil:
		//nolint:nilerr // malformed properties are reported by the consumer's own parsing
		return nil, nil
	}
	seen := map[string]ComponentRead{}
	for _, read := range plan.Reads(propexpr.ComponentIdent) {
		if len(read.Path) == 0 {
			continue
		}
		calls, path := propexpr.Placement(read.Reference)
		r := ComponentRead{Trait: trait, Producer: path[0], Placement: calls, Path: path[1:], Guarded: read.Defaulted}
		// An unguarded read of the same path wins: it is the one that waits.
		if prev, dup := seen[r.String()]; !dup || (prev.Guarded && !r.Guarded) {
			seen[r.String()] = r
		}
	}
	out := make([]ComponentRead, 0, len(seen))
	for _, r := range seen {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].String() < out[j].String() })
	return out, nil
}

// ReadDependencies is the components a component reads beside itself, naming no
// cluster or namespace: each is an implied dependsOn, with dependsOn's meaning of
// the producer in the reader's own placement. A read that names a placement
// implies none; the workflow orders it.
func ReadDependencies(comp common.ApplicationComponent, annotations map[string]string) []string {
	if !ExpressionsEnabledFor(annotations) {
		return nil
	}
	reads, err := ComponentReads(comp)
	if err != nil {
		// Admission refuses a read that cannot be parsed; the render reports it.
		return nil
	}
	seen := map[string]bool{}
	var out []string
	for _, r := range reads {
		if len(r.Placement) > 0 || r.Producer == comp.Name || seen[r.Producer] {
			continue
		}
		seen[r.Producer] = true
		out = append(out, r.Producer)
	}
	sort.Strings(out)
	return out
}

// EffectiveDependsOn is a component's dependsOn as written, then each component
// it reads beside itself that is not already there.
func EffectiveDependsOn(comp common.ApplicationComponent, annotations map[string]string) []string {
	out := slices.Clone(comp.DependsOn)
	for _, p := range ReadDependencies(comp, annotations) {
		if !slices.Contains(out, p) {
			out = append(out, p)
		}
	}
	return out
}

// Dependencies is what each component depends on, sorted and each once: the
// components named in its dependsOn, those whose outputs its inputs read, and,
// in an Application that has opted in, those its property expressions read, with
// the cluster and namespace an expression names.
func Dependencies(comps []common.ApplicationComponent, annotations map[string]string) []common.ComponentDependency {
	outputOwner := map[string]string{}
	for _, c := range comps {
		for _, o := range c.Outputs {
			outputOwner[o.Name] = c.Name
		}
	}
	expressions := ExpressionsEnabledFor(annotations)
	seen := map[common.ComponentDependency]bool{}
	var out []common.ComponentDependency
	add := func(d common.ComponentDependency) {
		if d.DependsOn != "" && d.DependsOn != d.Component && !seen[d] {
			seen[d] = true
			out = append(out, d)
		}
	}
	for _, c := range comps {
		for _, d := range c.DependsOn {
			add(common.ComponentDependency{Component: c.Name, DependsOn: d, Source: common.DependencySourceDependsOn})
		}
		for _, in := range c.Inputs {
			add(common.ComponentDependency{Component: c.Name, DependsOn: outputOwner[in.From], Source: common.DependencySourceInputs})
		}
		if !expressions {
			continue
		}
		reads, err := ComponentReads(c)
		if err != nil {
			// Admission refuses a read that cannot be parsed; the render reports it.
			continue
		}
		for _, r := range reads {
			t, err := r.Target()
			if err != nil {
				continue
			}
			add(common.ComponentDependency{Component: c.Name, DependsOn: r.Producer, Source: common.DependencySourceExpression,
				Cluster: t.Cluster, Namespace: t.Namespace})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		return strings.Join([]string{a.Component, a.DependsOn, string(a.Source), a.Cluster, a.Namespace}, "\x00") <
			strings.Join([]string{b.Component, b.DependsOn, string(b.Source), b.Cluster, b.Namespace}, "\x00")
	})
	return out
}

// DependencyPlacement is where an expression reads a component, as it named it:
// a cluster, a cluster and namespace, or a namespace of the reader's own cluster,
// said as such so that it is not taken for a cluster. It is empty for a
// dependency beside its component.
func DependencyPlacement(d common.ComponentDependency) string {
	switch {
	case d.Cluster != "" && d.Namespace != "":
		return d.Cluster + "/" + d.Namespace
	case d.Cluster != "":
		return d.Cluster
	case d.Namespace != "":
		return "namespace " + d.Namespace
	}
	return ""
}

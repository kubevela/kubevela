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

package appfile

import (
	"fmt"
	"strings"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/sources"
)

// ValidateComponentReads checks every component read in an Application against
// what a render can honour.
//
// A read names a component in the same Application other than the reader, and
// either its workload output or a named trait resource, those being what a
// component's output view holds. A placement is named straight after the
// component, with literal arguments. Reads, dependsOn and explicit inputs
// together must not form a cycle, which would leave every component in it
// waiting on another; a read naming a placement counts too, as its reader waits
// on it just the same.
func ValidateComponentReads(spec v1beta1.ApplicationSpec) error {
	comps := spec.Components
	names := map[string]bool{}
	for _, c := range comps {
		names[c.Name] = true
	}
	outputOwner := map[string]string{}
	for _, c := range comps {
		for _, o := range c.Outputs {
			outputOwner[o.Name] = c.Name
		}
	}

	edges := map[string][]string{}
	for _, c := range comps {
		reads, err := sources.ComponentReads(c)
		if err != nil {
			return fmt.Errorf("component %q: %w", c.Name, err)
		}
		for _, r := range reads {
			if err := validateRead(c.Name, r, names); err != nil {
				return err
			}
			edges[c.Name] = append(edges[c.Name], r.Producer)
		}
		edges[c.Name] = append(edges[c.Name], c.DependsOn...)
		for _, in := range c.Inputs {
			if owner, ok := outputOwner[in.From]; ok {
				edges[c.Name] = append(edges[c.Name], owner)
			}
		}
	}
	if cycle := findCycle(comps, edges); cycle != nil {
		return fmt.Errorf("components wait on each other in a cycle: %s", strings.Join(cycle, " -> "))
	}
	return nil
}

func validateRead(reader string, r sources.ComponentRead, names map[string]bool) error {
	switch {
	case !names[r.Producer]:
		return fmt.Errorf("component %q reads %s, but the application has no component %q", reader, r, r.Producer)
	case r.Producer == reader:
		return fmt.Errorf("component %q reads its own output (%s); a component's output only exists once it has been applied",
			reader, r)
	case r.Misplaced():
		return fmt.Errorf("component %q reads %s; cluster and namespace go straight after the component: "+
			"component.%s.cluster(\"<cluster>\").output", reader, r, r.Producer)
	}
	if _, err := r.Target(); err != nil {
		return fmt.Errorf("component %q reads %w", reader, err)
	}
	if !validOutputPath(r.Path) {
		return fmt.Errorf("component %q reads %s; read component.%s.output or component.%s.outputs.<resource>, "+
			"optionally at a placement named with literal strings: .cluster(\"<cluster>\")",
			reader, r, r.Producer, r.Producer)
	}
	return nil
}

func validOutputPath(path []string) bool {
	switch {
	case len(path) >= 1 && path[0] == "output":
		return true
	case len(path) >= 2 && path[0] == "outputs":
		return true
	}
	return false
}

// findCycle returns the first cycle found, as the names along it with the first
// repeated at the end, or nil. Components are visited in spec order so the
// report is stable.
func findCycle(comps []common.ApplicationComponent, edges map[string][]string) []string {
	const (
		unvisited = iota
		onStack
		done
	)
	state := map[string]int{}
	var stack []string
	var visit func(string) []string
	visit = func(n string) []string {
		state[n] = onStack
		stack = append(stack, n)
		for _, next := range edges[n] {
			switch state[next] {
			case onStack:
				for i, s := range stack {
					if s == next {
						return append(append([]string{}, stack[i:]...), next)
					}
				}
			case unvisited:
				if c := visit(next); c != nil {
					return c
				}
			}
		}
		stack = stack[:len(stack)-1]
		state[n] = done
		return nil
	}
	for _, c := range comps {
		if state[c.Name] == unvisited {
			if cycle := visit(c.Name); cycle != nil {
				return cycle
			}
		}
	}
	return nil
}

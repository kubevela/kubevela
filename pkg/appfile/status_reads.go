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
	"sort"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/cue/definition/health"
	"github.com/oam-dev/kubevela/pkg/definition/inherit"
)

// statusLevel is one ancestor's status CUE, by the name the chain reached it.
type statusLevel struct {
	name   string
	status *common.Status
}

// ancestorStatuses pairs a chain's ancestors, nearest first, with their status.
func ancestorStatuses(levels []inherit.Level, statusOf func(name string) *common.Status) []statusLevel {
	out := make([]statusLevel, 0, len(levels))
	for _, l := range levels {
		out = append(out, statusLevel{name: l.Name, status: statusOf(l.Name)})
	}
	return out
}

// statusConcern is one part of a status a child inherits: how to read it off a
// Status, whether a level's own snippet takes it over, and how to name it.
type statusConcern struct {
	field    string
	verb     string
	snippet  func(*common.Status) string
	takeover func(snippet string) bool
	how      string
}

var statusConcerns = []statusConcern{
	{
		field: "healthPolicy", verb: "reads",
		snippet:  func(s *common.Status) string { return s.HealthPolicy },
		takeover: func(sn string) bool { return !health.Inherits(sn) },
		how:      "take the health policy over with `$inherit: false` in its own healthPolicy",
	},
	{
		field: "customStatus", verb: "reads",
		snippet: func(s *common.Status) string { return s.CustomStatus },
		// A child's own message replaces the parent's.
		takeover: func(string) bool { return true },
		how:      "set its own customStatus",
	},
	{
		field: "details", verb: "read",
		snippet:  func(s *common.Status) string { return s.Details },
		takeover: func(sn string) bool { return !health.Inherits(sn) },
		how:      "take the details over with `$inherit: false` in its own details",
	},
}

// checkStatusReads refuses a definition that leaves out one of its parent's
// outputs while a status it still inherits reads that output. The status was
// written against the parent's output; evaluated against something else, or
// nothing, it reports on a resource it no longer describes.
//
// A level that takes a concern over, at the child or between it and the
// ancestor, ends the inheritance of that concern from above.
func checkStatusReads(kind, name, template string, own *common.Status, ancestors []statusLevel) error {
	dropped, err := inherit.DroppedOutputs(inherit.Level{Name: name, Template: template}, "outputs")
	if err != nil || len(dropped) == 0 {
		return err
	}
	drops := make([]string, 0, len(dropped))
	for k := range dropped {
		drops = append(drops, k)
	}
	sort.Strings(drops)

	taken := map[string]bool{}
	take := func(s *common.Status) {
		if s == nil {
			return
		}
		for _, c := range statusConcerns {
			if sn := c.snippet(s); sn != "" && c.takeover(sn) {
				taken[c.field] = true
			}
		}
	}
	take(own)
	for _, anc := range ancestors {
		if anc.status != nil {
			for _, c := range statusConcerns {
				if taken[c.field] {
					continue
				}
				keys, all := health.OutputsRead(c.snippet(anc.status))
				for _, k := range drops {
					if all || keys[k] {
						return fmt.Errorf("%s %s leaves out outputs.%s, which the %s it inherits from %s %s; keep the output, or %s",
							kind, name, k, c.field, anc.name, c.verb, c.how)
					}
				}
			}
		}
		take(anc.status)
	}
	return nil
}

// traitTemplateOf is the template a trait renders with, which may have come
// from spec.extension rather than the CUE schematic.
func traitTemplateOf(tmpl *Template, td *v1beta1.TraitDefinition) string {
	if tmpl.TemplateStr != "" {
		return tmpl.TemplateStr
	}
	if td.Spec.Schematic == nil || td.Spec.Schematic.CUE == nil {
		return ""
	}
	return td.Spec.Schematic.CUE.Template
}

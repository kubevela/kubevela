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

package cachekey

import (
	"fmt"
	"sort"
	"strings"

	"github.com/oam-dev/kubevela/pkg/definition/propexpr"
)

// RequiredContext returns the context fields a source template reads.
//
// Derived from the template rather than read back from $internal.keyInputs, so
// it cannot disagree with what the resolver will actually ask for - the same
// reason admission re-derives the key instead of trusting the stamped one.
func RequiredContext(template string) ([]string, error) {
	rules, err := LoadRules()
	if err != nil {
		return nil, err
	}
	dims, err := Infer(template, rules)
	if err != nil {
		return nil, err
	}

	seen := map[string]bool{}
	var fields []string
	for _, d := range dims {
		// The field, not the rendered dimension: appLabels[team] and
		// appLabels[region] are two reads of one field, and it is the field a
		// surface either offers or does not.
		if seen[d.Field] {
			continue
		}
		seen[d.Field] = true
		fields = append(fields, d.Field)
	}
	sort.Strings(fields)
	return fields, nil
}

// SurfacesSupporting returns the surfaces that can supply every field, out of
// those given.
func SurfacesSupporting(fields, surfaces []string) []string {
	var out []string
	for _, surface := range surfaces {
		if missingOn(fields, surface) == nil {
			out = append(out, surface)
		}
	}
	sort.Strings(out)
	return out
}

// missingOn lists the fields a surface does not offer.
func missingOn(fields []string, surface string) []string {
	if !propexpr.SurfaceDeclared(surface) {
		// An unrecognised surface offers everything rather than nothing: a
		// caller not yet taught to name itself must not start failing.
		return nil
	}
	var missing []string
	for _, field := range fields {
		// context.name is supplied from the binding, not the caller, so no
		// surface can withhold it.
		if field == contextNameField || propexpr.SurfaceOffers(surface, field) {
			continue
		}
		missing = append(missing, field)
	}
	return missing
}

// contextNameField is the binding entry - see KEP-2.16 amendment A4.
const contextNameField = "name"

// MissingOn names the context fields a surface cannot supply, as an author wrote
// them - context.componentName, not componentName.
//
// Exported alongside CheckSurface because a caller reporting per surface already
// knows which surface it is asking about, so the sentence CheckSurface builds
// ends in a clause it would only have to strip back off.
func MissingOn(fields []string, surface string) []string {
	missing := missingOn(fields, surface)
	if len(missing) == 0 {
		return nil
	}
	sort.Strings(missing)
	return dotted(missing)
}

// CheckSurface reports why a source reading these fields cannot resolve on a
// surface, or nil if it can.
func CheckSurface(fields []string, surface string) error {
	missing := missingOn(fields, surface)
	if len(missing) == 0 {
		return nil
	}
	sort.Strings(missing)
	return fmt.Errorf("reads %s, which is unavailable in %s",
		strings.Join(dotted(missing), ", "), propexpr.SurfacePlural(surface))
}

// dotted names a context field the way an author wrote it.
func dotted(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		out = append(out, "context."+s)
	}
	return out
}

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

import (
	"fmt"
	"strings"
)

// The shape of a read. It lives here rather than beside a parser because both the
// schema analysis in this package and whichever language expressions are written
// in have to agree on it.

// Reference is one path an expression reads, rooted at source or context.
type Reference struct {
	// Root is SourceIdent, ContextIdent or ComponentIdent.
	Root string
	// Path is the rest: for a source, [binding, field...]; for context,
	// [field] or [field, index].
	Path []string
	// Defaulted records that the read carries a fallback - *read | literal - so
	// it survives the value being absent.
	Defaulted bool
}

// IsSource reports whether the reference reads a resolved source.
func (r Reference) IsSource() bool { return r.Root == SourceIdent }

// IsComponent reports whether the reference reads another component's output.
func (r Reference) IsComponent() bool { return r.Root == ComponentIdent }

// Placement reports the placement calls a component read makes straight after
// the component, in order, and the path without them. Calls anywhere else stay
// in the path, where validation finds them.
func (r Reference) Placement() ([]string, []string) {
	if !r.IsComponent() || len(r.Path) < 2 {
		return nil, r.Path
	}
	var calls []string
	i := 1
	for ; i < len(r.Path); i++ {
		call, ok := SegmentQualifier(r.Path[i])
		if !ok {
			break
		}
		calls = append(calls, call)
	}
	return calls, append([]string{r.Path[0]}, r.Path[i:]...)
}

// String renders a reference the way an error message names it.
//
// Segments are rendered so the result is a valid expression, because the errors
// that use it tell the author what to write instead - "supply a default with
// *<ref> | <fallback>". A path joined with dots would suggest
// `source.cfg.outputs.0.name`, which does not parse, and the author would be
// left correcting the suggestion before they could take it.
func (r Reference) String() string {
	var b strings.Builder
	b.WriteString(r.Root)
	for _, segment := range r.Path {
		if q, ok := SegmentQualifier(segment); ok {
			b.WriteString(qualifierExpr(q))
			continue
		}
		switch {
		case isIndexSegment(segment):
			fmt.Fprintf(&b, "[%s]", segment)
		case isCUEIdent(segment):
			b.WriteString("." + segment)
		default:
			// A hyphenated source name or a label key with a dot in it. Bracket
			// syntax is the only form that reads these at all.
			fmt.Fprintf(&b, "[%q]", segment)
		}
	}
	return b.String()
}

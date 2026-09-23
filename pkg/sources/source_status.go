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
	"time"

	"github.com/pkg/errors"
)

// The phases a binding is reported in. Declared here because this package
// writes them; a consumer comparing against its own copy of the string would
// stop matching the moment one side was edited.
const (
	// PhaseResolved means a value is in hand, whether freshly fetched or served
	// from the store.
	PhaseResolved = "Resolved"
	// PhaseFailed means the binding has no value and the message says why.
	PhaseFailed = "Failed"
	// PhaseStale is reported by the Application controller for a value served
	// past its TTL, which this package reports as resolved with a reason.
	PhaseStale = "Stale"
	// PhaseUnused means the binding was declared and nothing read it.
	PhaseUnused = "Unused"
)

// SourceRead is one value taken from a source: what was read, where it went,
// and who read it.
type SourceRead struct {
	// SourceAttr is the attribute of the source that was read, e.g. "data.image".
	SourceAttr string
	// Property is the consumer's property it landed in, e.g. "image" or
	// "env[0].value". Empty when the read happened somewhere without a property
	// path, which today means a source resolving its own properties.
	Property string
	// ReaderKind and ReaderName name the reader when it is not the surface being
	// rendered - a source whose own properties read an earlier source. Empty
	// means the enclosing component, trait or step.
	ReaderKind string
	ReaderName string
	Value      interface{}
}

// SourceResolutionStatus is what one binding's resolution produced, for the
// caller to report.
type SourceResolutionStatus struct {
	Name      string
	Type      string
	Phase     string
	Message   string
	Config    string
	ExpiresAt string
	// ConsumedFields is field -> value, and is what the auto-update hash is
	// computed over. Deliberately left as a map: json.Marshal sorts map keys, so
	// the hash is stable, and moving it to an ordered list would risk every
	// existing workload's stamped hash changing on upgrade and re-dispatching.
	ConsumedFields map[string]interface{}
	// Reads is the same information with the destination attached: which property
	// of the consumer each value landed in, and which reader took it. Reporting
	// only, never hashed.
	Reads          []SourceRead
	SensitivePaths []string
}

func (r *sourceResolver) setSourceStatus(sourceName, sourceType, phase, message, config, expiresAt string) {
	statuses := r.statuses
	current := statuses[sourceName]
	consumed := current.ConsumedFields
	if consumed == nil {
		consumed = map[string]interface{}{}
	}
	statuses[sourceName] = SourceResolutionStatus{
		Name:           sourceName,
		Type:           sourceType,
		Phase:          phase,
		Message:        message,
		Config:         config,
		ExpiresAt:      expiresAt,
		ConsumedFields: consumed,
		SensitivePaths: append([]string{}, r.sensitivePaths[sourceType]...),
	}
}

func (r *sourceResolver) recordConsumedValue(sourceName, sourceType, path string, v interface{}, property string) {
	statuses := r.statuses
	st := statuses[sourceName]
	if st.Name == "" {
		st.Name = sourceName
	}
	if st.Type == "" {
		st.Type = sourceType
	}
	if st.ConsumedFields == nil {
		st.ConsumedFields = map[string]interface{}{}
	}
	st.ConsumedFields[path] = v
	st.Reads = append(st.Reads, SourceRead{
		SourceAttr: path,
		Property:   property,
		ReaderKind: r.readerKind,
		ReaderName: r.readerName,
		Value:      v,
	})
	if len(st.SensitivePaths) == 0 {
		st.SensitivePaths = append([]string{}, r.sensitivePaths[sourceType]...)
	}
	statuses[sourceName] = st
}

// mergeStatuses folds one render pass's statuses into what earlier passes on the
// same context recorded.
//
// A component and every one of its traits render against a single
// process.Context, one pass each, and each builds its status map from scratch.
// PushData replaces, so the merge is what keeps every pass's bindings - and with
// them the resolved-hash that drives auto-update, which is stamped only for
// bindings present in the final map.
//
// Later wins on resolution state, being the more recent answer. Consumption
// accumulates instead: each pass records a different reader taking a different
// property, and those are all true at once.
func mergeStatuses(prior, next map[string]SourceResolutionStatus) map[string]SourceResolutionStatus {
	if len(prior) == 0 {
		return next
	}
	out := make(map[string]SourceResolutionStatus, len(prior)+len(next))
	for name, st := range prior {
		out[name] = st
	}
	for name, cur := range next {
		before, seen := out[name]
		if !seen {
			out[name] = cur
			continue
		}
		out[name] = mergeStatus(before, cur)
	}
	return out
}

// mergeStatus combines two records of one binding: the newer resolution, and
// every read either pass made.
func mergeStatus(before, cur SourceResolutionStatus) SourceResolutionStatus {
	merged := cur

	// Consumption is cumulative. The newer value wins a conflict - both passes
	// read the same resolution, so they agree, and where they do not the later
	// one is the one the render actually used.
	if len(before.ConsumedFields) > 0 {
		fields := make(map[string]interface{}, len(before.ConsumedFields)+len(cur.ConsumedFields))
		for k, v := range before.ConsumedFields {
			fields[k] = v
		}
		for k, v := range cur.ConsumedFields {
			fields[k] = v
		}
		merged.ConsumedFields = fields
	}

	// Reads carry the reader and the property, so two passes contribute
	// different entries and both belong. Deduped on that identity rather than
	// appended blindly, so a binding re-resolved by the same reader does not
	// accumulate a duplicate.
	if len(before.Reads) > 0 {
		// An explicit key rather than SourceRead itself: SourceRead carries a
		// Value of type interface{}, and using it as a map key would panic the
		// moment a source resolved to a map or a list.
		type readKey struct{ attr, property, readerKind, readerName string }
		seen := make(map[readKey]bool, len(before.Reads)+len(cur.Reads))
		reads := make([]SourceRead, 0, len(before.Reads)+len(cur.Reads))
		for _, r := range append(append([]SourceRead{}, before.Reads...), cur.Reads...) {
			key := readKey{r.SourceAttr, r.Property, r.ReaderKind, r.ReaderName}
			if seen[key] {
				continue
			}
			seen[key] = true
			reads = append(reads, r)
		}
		merged.Reads = reads
	}

	// A pass that resolved from cache carries no sensitive paths of its own.
	if len(merged.SensitivePaths) == 0 {
		merged.SensitivePaths = before.SensitivePaths
	}
	return merged
}

// staleFallback is what a refresh needs in order to decide, when it cannot
// complete, whether a previously stored value may stand in for the answer.
type staleFallback struct {
	name       string
	sourceType string
	policy     sourceCachePolicy
	cached     map[string]interface{}
	found      bool
	stale      bool
	expiresAt  time.Time
}

// serveStale returns the stored value when a refresh has failed and the
// definition asked for that, reporting it as resolved with the reason it is not
// fresh. The second return says whether it applied.
//
// Written once because every refresh failure has to make the same decision, and
// each is also a place where forgetting to record status would leave a binding
// looking unresolved while its value was in use.
func (r *sourceResolver) serveStale(f staleFallback, reason string) (map[string]interface{}, bool) {
	if !f.found || !f.stale || f.policy.OnStaleFailure != sourceCachePolicyUseStale {
		return nil, false
	}
	r.touchSourceCache(f.policy.Key)
	r.resolved[f.name] = f.cached
	r.setSourceStatus(f.name, f.sourceType, PhaseResolved, reason,
		f.policy.Key, formatExpiry(f.expiresAt))
	return f.cached, true
}

// fail records a binding as failed and returns the error to hand back, so a
// resolution cannot report one without the other.
//
// The status carries the cause on its own and the returned error carries the
// context around it. A reader of status.sources[] is already looking at the
// binding that failed, so naming it again there says nothing; a caller further
// up has lost that, so the error says which step it was.
//
//nolint:unparam // the nil map is the values half of every caller's (values, error) return, so a failure stays one statement
func (r *sourceResolver) fail(name, sourceType, cacheKey string, err error, context string) (map[string]interface{}, error) {
	r.setSourceStatus(name, sourceType, PhaseFailed, err.Error(), cacheKey, "")
	if context == "" {
		return nil, err
	}
	return nil, errors.WithMessage(err, context)
}

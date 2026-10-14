/*
Copyright 2025 The KubeVela Authors.

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

package defkit

import (
	"encoding/json"
	"fmt"
	"strings"

	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/format"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
)

// MatchLabels returns a selector matching namespaces that carry every given label.
// Use a metav1.LabelSelector directly for matchExpressions.
func MatchLabels(labels map[string]string) *metav1.LabelSelector {
	return &metav1.LabelSelector{MatchLabels: labels}
}

// NamespaceQuota is one entry of a definition's per-namespace usage quota.
// The first entry matching a namespace applies; DefaultQuota matches every
// namespace and must come last.
type NamespaceQuota struct {
	q common.NamespaceQuota
}

// QuotaFor starts a quota entry for the namespaces matching any of the names or
// path.Match globs.
func QuotaFor(namespaces ...string) *NamespaceQuota {
	return &NamespaceQuota{q: common.NamespaceQuota{Namespaces: namespaces}}
}

// QuotaForSelector starts a quota entry for the namespaces matching the selector.
func QuotaForSelector(selector *metav1.LabelSelector) *NamespaceQuota {
	return &NamespaceQuota{q: common.NamespaceQuota{NamespaceSelector: selector}}
}

// DefaultQuota starts the quota entry for every namespace no earlier entry matched.
func DefaultQuota() *NamespaceQuota {
	return &NamespaceQuota{}
}

// Warn admits an Application with a warning once the namespace uses the
// definition this many times.
func (n *NamespaceQuota) Warn(count int32) *NamespaceQuota {
	n.q.Warn = &count
	return n
}

// Limit refuses an Application that would take the namespace's usage of the
// definition above this many. Zero forbids the definition in those namespaces.
func (n *NamespaceQuota) Limit(count int32) *NamespaceQuota {
	n.q.Limit = &count
	return n
}

// --- baseDefinition storage, exposed per kind ---

func (b *baseDefinition) ensureRestrictions() *common.DefinitionRestrictions {
	if b.restrictions == nil {
		b.restrictions = &common.DefinitionRestrictions{}
	}
	return b.restrictions
}

func (b *baseDefinition) addRestrictNamespaces(namespaces ...string) {
	r := b.ensureRestrictions()
	r.Namespaces = append(r.Namespaces, namespaces...)
}

func (b *baseDefinition) setRestrictNamespaceSelector(selector *metav1.LabelSelector) {
	b.ensureRestrictions().NamespaceSelector = selector
}

func (b *baseDefinition) addQuota(entries ...*NamespaceQuota) {
	r := b.ensureRestrictions()
	for _, e := range entries {
		r.Quota = append(r.Quota, e.q)
	}
}

// GetRestrictions returns the namespace restrictions and quota, or nil when none are set.
func (b *baseDefinition) GetRestrictions() *common.DefinitionRestrictions {
	return b.restrictions
}

// writeRestrictions writes the restrictions as an attribute, in the shape of the
// API type so the generated CUE cannot drift from the CRD.
func writeRestrictions(sb *strings.Builder, r *common.DefinitionRestrictions, indent string) {
	if r == nil {
		return
	}
	v := cuecontext.New().Encode(r)
	if v.Err() != nil {
		panic(fmt.Sprintf("defkit: encoding restrictions: %v", v.Err()))
	}
	formatted, err := format.Node(v.Syntax(), format.Simplify())
	if err != nil {
		panic(fmt.Sprintf("defkit: formatting restrictions: %v", err))
	}
	sb.WriteString(indent + "restrictions: ")
	lines := strings.Split(strings.TrimRight(string(formatted), "\n"), "\n")
	sb.WriteString(lines[0] + "\n")
	for _, line := range lines[1:] {
		sb.WriteString(indent + line + "\n")
	}
}

// setRestrictionsSpec copies the restrictions into a ToYAML spec.
func setRestrictionsSpec(spec map[string]any, r *common.DefinitionRestrictions) {
	if r == nil {
		return
	}
	bs, err := json.Marshal(r)
	if err != nil {
		panic(fmt.Sprintf("defkit: encoding restrictions: %v", err))
	}
	var m map[string]any
	if err := json.Unmarshal(bs, &m); err != nil {
		panic(fmt.Sprintf("defkit: encoding restrictions: %v", err))
	}
	spec["restrictions"] = m
}

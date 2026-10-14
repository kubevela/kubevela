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

package mcp

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

// Change actions.
const (
	ActionAdd    = "ADD"
	ActionModify = "MODIFY"
	ActionRemove = "REMOVE"
)

// maxFieldChanges bounds the field entries listed per resource.
const maxFieldChanges = 50

// FieldChange is one leaf that differs between the base and target render.
// Values are redacted and shortened; the path is a dotted JSON path with
// list indexes in brackets.
type FieldChange struct {
	Path string `json:"path"`
	From any    `json:"from,omitempty"`
	To   any    `json:"to,omitempty"`
}

// ResourceChange is one rendered object that differs.
type ResourceChange struct {
	Component  string        `json:"component,omitempty"`
	APIVersion string        `json:"apiVersion"`
	Kind       string        `json:"kind"`
	Namespace  string        `json:"namespace,omitempty"`
	Name       string        `json:"name"`
	Action     string        `json:"action"`
	Fields     []FieldChange `json:"fields,omitempty"`
	// FieldsTruncated is set when more than maxFieldChanges leaves differ.
	FieldsTruncated bool `json:"fieldsTruncated,omitempty"`
}

// ChangeSet is everything the classifier and the reader need about a
// proposed change. It is computed from two renders and two Application
// specs and holds no cluster handles, so it can be classified offline.
type ChangeSet struct {
	// NewApplication is true when no revision of the Application exists.
	NewApplication bool             `json:"newApplication"`
	Resources      []ResourceChange `json:"resources"`
	// ComponentsAdded and ComponentsRemoved compare the declared components.
	ComponentsAdded   []string `json:"componentsAdded,omitempty"`
	ComponentsRemoved []string `json:"componentsRemoved,omitempty"`
	// PlacementChanged is true when a topology policy changed or the
	// Application moved namespace.
	PlacementChanged bool `json:"placementChanged"`
	// GateRemoved is true when a suspend step that preceded the first deploy
	// step in the base is absent in the target.
	GateRemoved bool `json:"gateRemoved"`
	// WorkflowChanged is true when the declared workflow differs at all.
	WorkflowChanged bool `json:"workflowChanged"`
	// PoliciesChanged lists policy names whose type or properties changed.
	PoliciesChanged []string `json:"policiesChanged,omitempty"`
}

// ignoredLeaf lists paths that differ between every render and carry no
// intent: revision bookkeeping the controller stamps on each object.
var ignoredLeaf = []string{
	"metadata.labels.app.oam.dev/appRevision",
	"metadata.annotations.app.oam.dev/publishVersion",
	"metadata.annotations.app.oam.dev/deployVersion",
	"metadata.annotations.oam.dev/render-hash",
	"metadata.annotations.app.oam.dev/last-applied-time",
}

// buildChangeSet compares a base render (nil for a new Application) with
// the target render.
func buildChangeSet(base, target *rendered) ChangeSet {
	cs := ChangeSet{Resources: []ResourceChange{}}
	if base == nil {
		cs.NewApplication = true
	}
	owner := map[string]string{}
	for _, r := range target.resources {
		owner[resourceKey(r.Kind, r.Namespace, r.Name)] = r.Component
	}
	if base != nil {
		for _, r := range base.resources {
			if _, ok := owner[resourceKey(r.Kind, r.Namespace, r.Name)]; !ok {
				owner[resourceKey(r.Kind, r.Namespace, r.Name)] = r.Component
			}
		}
	}
	keys := map[string]bool{}
	for k := range target.objects {
		keys[k] = true
	}
	if base != nil {
		for k := range base.objects {
			keys[k] = true
		}
	}
	sorted := make([]string, 0, len(keys))
	for k := range keys {
		sorted = append(sorted, k)
	}
	sort.Strings(sorted)
	for _, key := range sorted {
		var oldObj, newObj *unstructured.Unstructured
		if base != nil {
			oldObj = base.objects[key]
		}
		newObj = target.objects[key]
		rc := compareObjects(oldObj, newObj)
		if rc == nil {
			continue
		}
		rc.Component = owner[key]
		cs.Resources = append(cs.Resources, *rc)
	}
	if base != nil {
		cs.ComponentsAdded, cs.ComponentsRemoved = componentDelta(base.app, target.app)
		cs.PlacementChanged = placementChanged(base.app, target.app)
		cs.PoliciesChanged = policiesChanged(base.app, target.app)
		cs.WorkflowChanged = !reflect.DeepEqual(base.app.Spec.Workflow, target.app.Spec.Workflow)
		cs.GateRemoved = hasGate(base.app) && !hasGate(target.app)
	}
	return cs
}

// compareObjects returns nil when the objects are equal after ignoring
// revision bookkeeping.
func compareObjects(oldObj, newObj *unstructured.Unstructured) *ResourceChange {
	var ref *unstructured.Unstructured
	action := ActionModify
	switch {
	case oldObj == nil && newObj == nil:
		return nil
	case oldObj == nil:
		action, ref = ActionAdd, newObj
	case newObj == nil:
		action, ref = ActionRemove, oldObj
	default:
		ref = newObj
	}
	rc := &ResourceChange{APIVersion: ref.GetAPIVersion(), Kind: ref.GetKind(), Namespace: ref.GetNamespace(), Name: ref.GetName(), Action: action}
	if action != ActionModify {
		return rc
	}
	oldLeaves, newLeaves := leaves(oldObj.Object), leaves(newObj.Object)
	paths := map[string]bool{}
	for p := range oldLeaves {
		paths[p] = true
	}
	for p := range newLeaves {
		paths[p] = true
	}
	sortedPaths := make([]string, 0, len(paths))
	for p := range paths {
		if isIgnoredLeaf(p) {
			continue
		}
		sortedPaths = append(sortedPaths, p)
	}
	sort.Strings(sortedPaths)
	for _, p := range sortedPaths {
		from, hadFrom := oldLeaves[p]
		to, hadTo := newLeaves[p]
		if hadFrom && hadTo && reflect.DeepEqual(from, to) {
			continue
		}
		if len(rc.Fields) >= maxFieldChanges {
			rc.FieldsTruncated = true
			break
		}
		rc.Fields = append(rc.Fields, FieldChange{Path: p, From: redactLeaf(p, from), To: redactLeaf(p, to)})
	}
	if len(rc.Fields) == 0 && !rc.FieldsTruncated {
		return nil
	}
	return rc
}

// leaves flattens an object into path to scalar value. Lists are indexed,
// so a reordered list reads as a change, which for Kubernetes containers
// and env it is.
func leaves(v any) map[string]any {
	out := map[string]any{}
	var walk func(prefix string, v any)
	walk = func(prefix string, v any) {
		switch t := v.(type) {
		case map[string]any:
			if len(t) == 0 {
				out[prefix] = map[string]any{}
				return
			}
			for k, val := range t {
				walk(joinPath(prefix, k), val)
			}
		case []any:
			if len(t) == 0 {
				out[prefix] = []any{}
				return
			}
			for i, val := range t {
				walk(fmt.Sprintf("%s[%d]", prefix, i), val)
			}
		default:
			out[prefix] = v
		}
	}
	walk("", v)
	return out
}

func joinPath(prefix, k string) string {
	if prefix == "" {
		return k
	}
	return prefix + "." + k
}

func isIgnoredLeaf(p string) bool {
	for _, ig := range ignoredLeaf {
		if p == ig {
			return true
		}
	}
	return strings.HasPrefix(p, "metadata.managedFields") || p == "metadata.resourceVersion" || p == "metadata.uid" || p == "metadata.generation" || p == "metadata.creationTimestamp"
}

// redactLeaf applies the key and value redaction rules to one leaf.
func redactLeaf(path string, v any) any {
	if v == nil {
		return nil
	}
	last := path
	if i := strings.LastIndex(path, "."); i >= 0 {
		last = path[i+1:]
	}
	if secretKey.MatchString(last) || strings.Contains(path, "stringData") || strings.HasSuffix(path, "].value") && envNameIsSecret(path) {
		return redactedValue
	}
	if s, ok := v.(string); ok {
		return text(s)
	}
	raw, err := json.Marshal(v)
	if err == nil && len(raw) > maxTextLen {
		return string(raw[:maxTextLen]) + truncMark
	}
	return v
}

// envNameIsSecret cannot see the sibling name from a leaf path, so env
// values are left to the value patterns; it exists so the rule above reads
// as intended and can grow when the comparison carries siblings.
func envNameIsSecret(string) bool { return false }

func componentDelta(oldApp, newApp *v1beta1.Application) (added, removed []string) {
	oldNames, newNames := map[string]bool{}, map[string]bool{}
	for _, c := range oldApp.Spec.Components {
		oldNames[c.Name] = true
	}
	for _, c := range newApp.Spec.Components {
		newNames[c.Name] = true
		if !oldNames[c.Name] {
			added = append(added, c.Name)
		}
	}
	for _, c := range oldApp.Spec.Components {
		if !newNames[c.Name] {
			removed = append(removed, c.Name)
		}
	}
	return added, removed
}

func placementChanged(oldApp, newApp *v1beta1.Application) bool {
	if oldApp.Namespace != newApp.Namespace {
		return true
	}
	return !reflect.DeepEqual(policiesOfType(oldApp, "topology"), policiesOfType(newApp, "topology"))
}

func policiesOfType(app *v1beta1.Application, t string) map[string]string {
	out := map[string]string{}
	for _, p := range app.Spec.Policies {
		if p.Type != t {
			continue
		}
		props := ""
		if p.Properties != nil {
			props = string(p.Properties.Raw)
		}
		out[p.Name] = props
	}
	return out
}

func policiesChanged(oldApp, newApp *v1beta1.Application) []string {
	index := func(app *v1beta1.Application) map[string]string {
		out := map[string]string{}
		for _, p := range app.Spec.Policies {
			props := ""
			if p.Properties != nil {
				props = string(p.Properties.Raw)
			}
			out[p.Name] = p.Type + ":" + props
		}
		return out
	}
	oldP, newP := index(oldApp), index(newApp)
	changed := map[string]bool{}
	for name, v := range oldP {
		if newP[name] != v {
			changed[name] = true
		}
	}
	for name, v := range newP {
		if oldP[name] != v {
			changed[name] = true
		}
	}
	names := make([]string, 0, len(changed))
	for n := range changed {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// hasGate reports whether a suspend step precedes the first deploy step,
// which is the shape gated mode relies on.
func hasGate(app *v1beta1.Application) bool {
	if app.Spec.Workflow == nil {
		return false
	}
	for _, s := range app.Spec.Workflow.Steps {
		switch s.Type {
		case "suspend":
			return true
		case "deploy", "apply-component", "apply-application":
			return false
		}
	}
	return false
}

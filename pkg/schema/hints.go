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

package schema

import (
	"strconv"
	"strings"

	"cuelang.org/go/cue"

	"github.com/oam-dev/kubevela/pkg/appfile"
	cueutils "github.com/oam-dev/kubevela/pkg/cue"
)

// UIHintPrefix starts a doc comment line that tells VelaUX how to render a
// parameter, one hint per line: `// +ui:format=table`, `// +ui:advanced`.
const UIHintPrefix = "+ui:"

// ExtensionUI is the OpenAPI extension carrying a property's UI hints.
const ExtensionUI = "x-vela-ui"

// UIHints are the `+ui:` markers of a parameter.
type UIHints struct {
	// Type names the VelaUX widget, overriding the one the shape implies.
	Type string `json:"type,omitempty"`
	// Format is how a list of structs is laid out: `table` for one row each.
	Format string `json:"format,omitempty"`
	// RowKey names the field that identifies a row of a list: unique, and
	// the row's title.
	RowKey string `json:"rowKey,omitempty"`
	// ItemLabel names the field that titles each item of a list.
	ItemLabel   string `json:"itemLabel,omitempty"`
	Label       string `json:"label,omitempty"`
	Placeholder string `json:"placeholder,omitempty"`
	// Error is the message shown when a value fails the parameter's
	// constraints, in place of one naming the raw constraint.
	Error string `json:"error,omitempty"`
	// Section names the collapsible section of the form the parameter is
	// shown in; the value keeps its place in the parameter.
	Section string `json:"section,omitempty"`
	// OptionsFrom names where the parameter's choices are read when the form
	// opens: configs:<template>, clusters or envs.
	OptionsFrom string `json:"optionsFrom,omitempty"`
	// Expression is `never` for a parameter that must be written as a
	// literal, even in an Application that reads $( ) expressions.
	Expression string   `json:"expression,omitempty"`
	Suggest    []string `json:"suggest,omitempty"`
	ColSpan    int      `json:"colSpan,omitempty"`
	// Order places the parameter among its siblings, ahead of declaration
	// order.
	Order *int `json:"order,omitempty"`
	// Advanced puts the parameter behind VelaUX's Advanced toggle.
	Advanced bool `json:"advanced,omitempty"`
	Hidden   bool `json:"hidden,omitempty"`
}

func (h UIHints) empty() bool {
	return h.Type == "" && h.Format == "" && h.RowKey == "" && h.ItemLabel == "" && h.Label == "" &&
		h.Placeholder == "" && h.Error == "" && h.Section == "" && h.OptionsFrom == "" && h.Expression == "" &&
		len(h.Suggest) == 0 && h.ColSpan == 0 && h.Order == nil && !h.Advanced && !h.Hidden
}

// doc is what a parameter's doc comment says.
type doc struct {
	description string
	immutable   bool
	ui          UIHints
}

func docOf(v cue.Value) doc {
	var parts []string
	for _, cg := range v.Doc() {
		parts = append(parts, cg.Text())
	}
	return parseDoc(strings.Join(parts, "\n"))
}

// parseDoc reads a doc comment: the description after +usage=, cut at
// +short, with the +immutable and +ui: lines lifted out. +ignore hides a
// parameter from the CLI only, so here its line is dropped and the field kept.
func parseDoc(text string) doc {
	var d doc
	var kept []string
	for _, line := range strings.Split(text, "\n") {
		t := strings.TrimSpace(line)
		switch {
		case t == appfile.ImmutableTag:
			d.immutable = true
		case t == cueutils.IgnorePrefix:
		case strings.HasPrefix(t, UIHintPrefix):
			d.ui.set(strings.TrimPrefix(t, UIHintPrefix))
		default:
			kept = append(kept, line)
		}
	}
	desc := strings.TrimSpace(strings.Join(kept, "\n"))
	if strings.Contains(desc, appfile.UsageTag) {
		desc = strings.Split(desc, appfile.UsageTag)[1]
	}
	if strings.Contains(desc, appfile.ShortTag) {
		desc = strings.Split(desc, appfile.ShortTag)[0]
	}
	d.description = strings.TrimSpace(desc)
	return d
}

// set applies one `key=value` or bare `key` hint. An unknown key is ignored,
// so a template written for a newer VelaUX still renders on an older one.
func (h *UIHints) set(hint string) {
	key, value, _ := strings.Cut(hint, "=")
	key, value = strings.TrimSpace(key), strings.TrimSpace(value)
	switch key {
	case "type":
		h.Type = value
	case "format":
		h.Format = value
	case "rowKey":
		h.RowKey = value
	case "itemLabel":
		h.ItemLabel = value
	case "label":
		h.Label = value
	case "placeholder":
		h.Placeholder = value
	case "error":
		h.Error = value
	case "section":
		h.Section = value
	case "optionsFrom":
		h.OptionsFrom = value
	case "expression":
		h.Expression = value
	case "suggest":
		for _, s := range strings.Split(value, ",") {
			if s = strings.TrimSpace(s); s != "" {
				h.Suggest = append(h.Suggest, s)
			}
		}
	case "colSpan":
		if n, err := strconv.Atoi(value); err == nil {
			h.ColSpan = n
		}
	case "order":
		if n, err := strconv.Atoi(value); err == nil {
			h.Order = &n
		}
	case "advanced":
		h.Advanced = value == "" || value == "true"
	case "hidden":
		h.Hidden = value == "" || value == "true"
	}
}

// apply sets the field's description, immutability and hints from a doc
// comment, keeping suggestions the shape already gave.
func (f *Field) apply(d doc) {
	f.Description, f.Immutable = d.description, d.immutable
	suggest := f.UI.Suggest
	f.UI = d.ui
	for _, s := range suggest {
		if !containsString(f.UI.Suggest, s) {
			f.UI.Suggest = append(f.UI.Suggest, s)
		}
	}
}

func containsString(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

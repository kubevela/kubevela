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

package application

import (
	"context"
	"encoding/json"
	"strings"

	"cuelang.org/go/cue"
	celengine "github.com/kubevela/pkg/cel"
	"github.com/kubevela/pkg/cel/template"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/definition/celexpr"
	"github.com/oam-dev/kubevela/pkg/definition/propexpr"
	"github.com/oam-dev/kubevela/pkg/schema"
)

// producedSchemas is what each component of the Application applies, as its
// component.<name> reads see it.
func (h *ValidatingHandler) producedSchemas(ctx context.Context, app *v1beta1.Application) map[string]schema.Produced {
	return schema.ProducedFor(ctx, h.Client, app.Namespace, app.Spec.Components, app.GetAnnotations())
}

// loopItemKind is the kind at a path within one item of a forEach loop, given its
// items: a literal list is typed as written, and one expression that is exactly
// a read is typed by what that read declares one element down. Anything else, a
// computed list or items from a workflow variable, declares nothing.
func loopItemKind(items []byte, readKind func(template.Reference) (cue.Kind, bool)) func([]string) (cue.Kind, bool) {
	if len(items) == 0 {
		return nil
	}
	var literal interface{}
	if err := json.Unmarshal(items, &literal); err != nil {
		return nil
	}
	switch v := literal.(type) {
	case []interface{}:
		s := schema.LiteralSchema(v)
		return func(path []string) (cue.Kind, bool) { return schema.KindAt(s, append([]string{"0"}, path...)) }
	case string:
		ref, ok := bareRead(v)
		if !ok || ref.Root == propexpr.EachIdent {
			return nil
		}
		return func(path []string) (cue.Kind, bool) {
			elem := append(append(append([]string{}, ref.Path...), "0"), path...)
			return readKind(template.Reference{Root: ref.Root, Path: elem})
		}
	}
	return nil
}

// bareRead is the read a value consists of when it is one $( ) expression doing
// nothing but read, such as $(source.cfg.regions).
func bareRead(value string) (template.Reference, bool) {
	parsed, err := template.Parse(value)
	if err != nil || !parsed.HasExpr() || !parsed.Whole() {
		return template.Reference{}, false
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return template.Reference{}, false
	}
	plan, err := celexpr.Vela.PlanJSON(raw)
	if err != nil {
		return template.Reference{}, false
	}
	var reads []celengine.Read
	for _, root := range celexpr.Roots {
		reads = append(reads, plan.Reads(root.Name)...)
	}
	if len(reads) != 1 {
		return template.Reference{}, false
	}
	ref := reads[0].Reference
	expr := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(value, "$("), ")"))
	if ref.Defaulted || expr != ref.String() {
		return template.Reference{}, false
	}
	return ref, true
}

// readKinds is the kind admission knows for a read in one blob: a source's from
// its schema, context's from the registry, a component's from what it applies,
// and the loop's from the items of the forEach the blob runs in.
func (t *typing) readKinds(bp blobPlan) func(celengine.Read) (cue.Kind, bool) {
	var ofRef func(template.Reference) (cue.Kind, bool)
	ofRef = func(ref template.Reference) (cue.Kind, bool) {
		switch ref.Root {
		case propexpr.ComponentIdent:
			return schema.ComponentReadKind(t.producedSchemas(), ref)
		case propexpr.EachIdent:
			return schema.EachReadKind(loopItemKind(bp.loopItems, ofRef), ref)
		}
		return t.compiled.Kind(ref)
	}
	return func(r celengine.Read) (cue.Kind, bool) { return ofRef(r.Reference) }
}

// producedSchemas loads what the Application's components apply the first time a
// component read is typed, so one that reads none loads nothing.
func (t *typing) producedSchemas() map[string]schema.Produced {
	if t.produced == nil {
		t.produced = map[string]schema.Produced{}
		if t.loadProduced != nil {
			t.produced = t.loadProduced()
		}
	}
	return t.produced
}

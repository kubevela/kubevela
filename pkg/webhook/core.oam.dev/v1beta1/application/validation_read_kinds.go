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

	"cuelang.org/go/cue"
	celengine "github.com/kubevela/pkg/cel"
	"github.com/kubevela/pkg/cel/template"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/definition/propexpr"
	"github.com/oam-dev/kubevela/pkg/schema"
)

// producedSchemas is what each component of the Application applies, as its
// component.<name> reads see it.
func (h *ValidatingHandler) producedSchemas(ctx context.Context, app *v1beta1.Application) map[string]schema.Produced {
	return schema.ProducedFor(ctx, h.Client, app.Namespace, app.Spec.Components, app.GetAnnotations())
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
			return schema.EachReadKind(schema.LoopItemKind(bp.loopItems, ofRef), ref)
		}
		return t.compiled.Kind(ref)
	}
	return func(r celengine.Read) (cue.Kind, bool) { return ofRef(r.Reference) }
}

// mayBeAbsent is the has()-default rule's judge for a read in one blob: a read
// that may be missing, unguarded, is refused where it feeds a required
// parameter. A source's and context's come from their schemas, and the loop's
// from the items of the forEach the blob runs in.
func (t *typing) mayBeAbsent(bp blobPlan) func(celengine.Read) bool {
	var ofRef func(template.Reference) bool
	ofRef = func(ref template.Reference) bool {
		if ref.Root == propexpr.EachIdent {
			return schema.EachMayBeAbsent(schema.LoopItemMayBeAbsent(bp.loopItems, ofRef), ref)
		}
		may, err := t.compiled.CanBeAbsent(ref)
		return err == nil && may
	}
	return func(r celengine.Read) bool { return ofRef(r.Reference) }
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

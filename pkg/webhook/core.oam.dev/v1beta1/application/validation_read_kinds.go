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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"sync"

	"cuelang.org/go/cue"
	"github.com/getkin/kin-openapi/openapi3"
	celengine "github.com/kubevela/pkg/cel"
	"github.com/kubevela/pkg/cel/template"
	"k8s.io/klog/v2"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/appfile"
	"github.com/oam-dev/kubevela/pkg/definition/celexpr"
	"github.com/oam-dev/kubevela/pkg/definition/inherit"
	"github.com/oam-dev/kubevela/pkg/definition/propexpr"
	oamutil "github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/pkg/schema"
)

// producedSchemas is what each component of the Application applies, by name,
// as its component.<name> reads see it: its definition's output schemas, at the
// revision its type names and with what it extends, and the resources its traits
// add. A trait that patches the workload leaves it untyped, one that patches the
// resources leaves them untyped, and a trait that cannot be read may do either.
// A component whose definition cannot be read is left out, so its reads fit
// anything.
func (h *ValidatingHandler) producedSchemas(ctx context.Context, app *v1beta1.Application) map[string]schema.Produced {
	out := map[string]schema.Produced{}
	lookupCtx := oamutil.SetNamespaceInCtx(ctx, app.Namespace)
	for _, comp := range app.Spec.Components {
		cd := &v1beta1.ComponentDefinition{}
		if err := oamutil.GetCapabilityDefinition(lookupCtx, h.Client, cd, comp.Type, app.GetAnnotations()); err != nil {
			continue
		}
		own := h.definitionOutputs(ctx, cd.Name, cueTemplate(cd.Spec.Schematic), func() ([]inherit.Level, error) {
			if cd.Spec.Extends == "" {
				return nil, nil
			}
			return appfile.ComponentAncestors(ctx, h.Client, cd)
		})
		if own == nil {
			continue
		}
		p := schema.Produced{Output: own.Output, Outputs: map[string]*openapi3.Schema{}}
		for name, s := range own.Outputs {
			p.Outputs[name] = s
		}
		patchesOutput, patchesOutputs := false, false
		for _, tr := range comp.Traits {
			added, output, outputs := h.traitEffect(lookupCtx, ctx, tr, app.GetAnnotations())
			patchesOutput, patchesOutputs = patchesOutput || output, patchesOutputs || outputs
			for name, s := range added {
				if _, taken := p.Outputs[name]; taken {
					// Two declarations of one resource: which is applied is not known here.
					s = nil
				}
				p.Outputs[name] = s
			}
		}
		if patchesOutput {
			p.Output = nil
		}
		if patchesOutputs {
			p.Outputs = nil
		}
		out[comp.Name] = p
	}
	return out
}

// traitEffect is what a trait does to its component's reads: the resources it
// adds, and whether it patches the workload and the other resources. A trait
// whose definition cannot be read may patch either.
func (h *ValidatingHandler) traitEffect(lookupCtx, ctx context.Context, tr common.ApplicationTrait,
	annotations map[string]string) (added map[string]*openapi3.Schema, output, outputs bool) {
	td := &v1beta1.TraitDefinition{}
	if err := oamutil.GetCapabilityDefinition(lookupCtx, h.Client, td, tr.Type, annotations); err != nil {
		return nil, true, true
	}
	tmpl := cueTemplate(td.Spec.Schematic)
	if tmpl == "" {
		return nil, true, true
	}
	output, outputs = schema.Patches(tmpl)
	own := h.definitionOutputs(ctx, td.Name, tmpl, func() ([]inherit.Level, error) {
		if td.Spec.Extends == "" {
			return nil, nil
		}
		return appfile.TraitAncestors(ctx, h.Client, td)
	})
	if own == nil {
		return nil, output, outputs
	}
	return own.Outputs, output, outputs
}

// definitionOutputs is a definition's output schemas over its extends chain, or
// nil when they cannot be generated.
func (h *ValidatingHandler) definitionOutputs(ctx context.Context, name, tmpl string,
	ancestors func() ([]inherit.Level, error)) *schema.OutputSchemas {
	if tmpl == "" {
		return nil
	}
	levels := []inherit.Level{{Name: name, Template: tmpl}}
	more, err := ancestors()
	if err != nil {
		klog.V(4).InfoS("skip typing component reads: definition chain unreadable", "definition", name, "err", err)
		return nil
	}
	levels = append(levels, more...)
	return outputSchemaCache.get(ctx, levels)
}

func cueTemplate(s *common.Schematic) string {
	if s == nil || s.CUE == nil {
		return ""
	}
	return s.CUE.Template
}

// outputSchemaCache keeps generated output schemas by the templates they came
// from, since admission sees the same few definitions over and over and a
// generation evaluates CUE. A template that cannot be generated is kept as nil.
var outputSchemaCache = &schemaCache{entries: map[string]*schema.OutputSchemas{}}

// maxCachedSchemas bounds the cache; past it, it starts again.
const maxCachedSchemas = 512

type schemaCache struct {
	mu      sync.Mutex
	entries map[string]*schema.OutputSchemas
}

func (c *schemaCache) get(ctx context.Context, levels []inherit.Level) *schema.OutputSchemas {
	sum := sha256.New()
	for _, l := range levels {
		sum.Write([]byte(l.Name))
		sum.Write([]byte{0})
		sum.Write([]byte(l.Template))
		sum.Write([]byte{0})
	}
	key := hex.EncodeToString(sum.Sum(nil))
	c.mu.Lock()
	s, ok := c.entries[key]
	c.mu.Unlock()
	if ok {
		return s
	}
	s, err := schema.ChainOutputSchemas(ctx, levels)
	if err != nil {
		klog.V(4).InfoS("skip typing component reads: output schemas not generated", "definition", levels[0].Name, "err", err)
		s = nil
	}
	c.mu.Lock()
	if len(c.entries) >= maxCachedSchemas {
		c.entries = map[string]*schema.OutputSchemas{}
	}
	c.entries[key] = s
	c.mu.Unlock()
	return s
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

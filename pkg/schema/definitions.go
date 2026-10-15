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
	"context"
	"crypto/sha256"
	"encoding/hex"
	"sync"

	"cuelang.org/go/cue/ast"
	cueformat "cuelang.org/go/cue/format"
	cueparser "cuelang.org/go/cue/parser"
	"github.com/getkin/kin-openapi/openapi3"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	k8sschema "k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/appfile"
	"github.com/oam-dev/kubevela/pkg/cue/upgrade"
	"github.com/oam-dev/kubevela/pkg/definition/inherit"
	oamutil "github.com/oam-dev/kubevela/pkg/oam/util"
)

// These read the definitions an Application's expressions are typed against, the
// way its render finds them: in the Application's namespace, then vela-system,
// at the revision a type names (`type@vN`). Admission and VelaUX's expression
// check both use them, so the two judge a read against the same schema.

// SourceDefinitionFor is the SourceDefinition a source of sourceType reads.
func SourceDefinitionFor(ctx context.Context, cli client.Reader, namespace, sourceType string,
	annotations map[string]string) (*v1beta1.SourceDefinition, error) {
	def := &v1beta1.SourceDefinition{}
	if err := oamutil.GetCapabilityDefinition(oamutil.SetNamespaceInCtx(ctx, namespace), cli, def, sourceType, annotations); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, apierrors.NewNotFound(k8sschema.GroupResource{Group: v1beta1.Group, Resource: "sourcedefinitions"}, sourceType)
		}
		return nil, err
	}
	return def, nil
}

// SourceSchemaText is the `schema:` block of the SourceDefinition a source of
// sourceType reads, as CUE text: what $(source.<name>) reads. It is "" for a
// definition that declares none.
func SourceSchemaText(ctx context.Context, cli client.Reader, namespace, sourceType string,
	annotations map[string]string) (string, error) {
	def, err := SourceDefinitionFor(ctx, cli, namespace, sourceType, annotations)
	if err != nil {
		return "", err
	}
	if def.Spec.Schematic == nil || def.Spec.Schematic.CUE == nil {
		return "", nil
	}
	return TopLevelBlock(def.Spec.Schematic.CUE.Template, "schema")
}

// TopLevelBlock is the CUE source of a template's top-level field named name, or
// "" when it has none. The template is parsed, not evaluated.
func TopLevelBlock(template, name string) (string, error) {
	file, err := cueparser.ParseFile("-", template, cueparser.ParseComments)
	if err != nil {
		return "", err
	}
	for _, decl := range file.Decls {
		field, ok := decl.(*ast.Field)
		if !ok {
			continue
		}
		label, _, err := ast.LabelName(field.Label)
		if err != nil || label != name {
			continue
		}
		bt, err := cueformat.Node(field.Value)
		if err != nil {
			return "", err
		}
		return string(bt), nil
	}
	return "", nil
}

// ProducedFor is what each of components applies, by name, as its
// component.<name> reads see it: its definition's output schemas, at the
// revision its type names and with what it extends, and the resources its traits
// add. A trait that patches the workload leaves it untyped, one that patches the
// resources leaves them untyped, and a trait that cannot be read may do either.
// A resource declared twice is untyped, since which one is applied is not known.
// A component whose definition cannot be read is left out, so its reads fit
// anything.
func ProducedFor(ctx context.Context, cli client.Client, namespace string, components []common.ApplicationComponent,
	annotations map[string]string) map[string]Produced {
	out := map[string]Produced{}
	lookupCtx := oamutil.SetNamespaceInCtx(ctx, namespace)
	for _, comp := range components {
		cd := &v1beta1.ComponentDefinition{}
		if err := oamutil.GetCapabilityDefinition(lookupCtx, cli, cd, comp.Type, annotations); err != nil {
			continue
		}
		own := definitionOutputs(ctx, cd.Name, upgrade.ComponentKind, cueTemplate(cd.Spec.Schematic), func() ([]inherit.Level, error) {
			if cd.Spec.Extends == "" {
				return nil, nil
			}
			return appfile.ComponentAncestors(ctx, cli, cd)
		})
		if own == nil {
			continue
		}
		p := Produced{Output: own.Output, Outputs: map[string]*openapi3.Schema{}}
		for name, s := range own.Outputs {
			p.Outputs[name] = s
		}
		patchesOutput, patchesOutputs := false, false
		for _, tr := range comp.Traits {
			added, output, outputs := traitEffect(lookupCtx, ctx, cli, tr, annotations)
			patchesOutput, patchesOutputs = patchesOutput || output, patchesOutputs || outputs
			for name, s := range added {
				if _, taken := p.Outputs[name]; taken {
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
// adds, and whether it patches the workload and the other resources, at any
// level of its extends chain. A trait whose chain cannot be read may patch either.
func traitEffect(lookupCtx, ctx context.Context, cli client.Client, tr common.ApplicationTrait,
	annotations map[string]string) (added map[string]*openapi3.Schema, output, outputs bool) {
	td := &v1beta1.TraitDefinition{}
	if err := oamutil.GetCapabilityDefinition(lookupCtx, cli, td, tr.Type, annotations); err != nil {
		return nil, true, true
	}
	tmpl := cueTemplate(td.Spec.Schematic)
	if tmpl == "" {
		return nil, true, true
	}
	var ancestors []inherit.Level
	if td.Spec.Extends != "" {
		var err error
		if ancestors, err = appfile.TraitAncestors(ctx, cli, td); err != nil {
			klog.V(4).InfoS("skip typing component reads: trait chain unreadable", "definition", td.Name, "err", err)
			return nil, true, true
		}
	}
	output, outputs = Patches(tmpl)
	for _, l := range ancestors {
		o, os := Patches(l.Template)
		output, outputs = output || o, outputs || os
	}
	own := definitionOutputs(ctx, td.Name, upgrade.TraitKind, tmpl, func() ([]inherit.Level, error) {
		return ancestors, nil
	})
	if own == nil {
		return nil, output, outputs
	}
	return own.Outputs, output, outputs
}

// definitionOutputs is a definition's output schemas over its extends chain, or
// nil when they cannot be generated. Its own template is put through the CUE
// compatibility pass its render would be, as its ancestors' already are.
func definitionOutputs(ctx context.Context, name string, kind upgrade.DefinitionKind, tmpl string,
	ancestors func() ([]inherit.Level, error)) *OutputSchemas {
	if tmpl == "" {
		return nil
	}
	tmpl, _ = upgrade.EnsureCueVersionCompatibility(tmpl, name, kind, upgrade.TemplateAreaMain)
	levels := []inherit.Level{{Name: name, Template: tmpl}}
	more, err := ancestors()
	if err != nil {
		klog.V(4).InfoS("skip typing component reads: definition chain unreadable", "definition", name, "err", err)
		return nil
	}
	return outputSchemaCache.get(ctx, append(levels, more...))
}

func cueTemplate(s *common.Schematic) string {
	if s == nil || s.CUE == nil {
		return ""
	}
	return s.CUE.Template
}

// outputSchemaCache keeps generated output schemas by the templates they came
// from: the same few definitions are typed against over and over, and a
// generation evaluates CUE. A template that cannot be generated is kept as nil.
var outputSchemaCache = &schemaCache{entries: map[string]*OutputSchemas{}}

// maxCachedSchemas bounds the cache; past it, it starts again.
const maxCachedSchemas = 512

type schemaCache struct {
	mu      sync.Mutex
	entries map[string]*OutputSchemas
}

func (c *schemaCache) get(ctx context.Context, levels []inherit.Level) *OutputSchemas {
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
	s, err := ChainOutputSchemas(ctx, levels)
	if err != nil {
		klog.V(4).InfoS("skip typing component reads: output schemas not generated", "definition", levels[0].Name, "err", err)
		s = nil
	}
	c.mu.Lock()
	if len(c.entries) >= maxCachedSchemas {
		c.entries = map[string]*OutputSchemas{}
	}
	c.entries[key] = s
	c.mu.Unlock()
	return s
}

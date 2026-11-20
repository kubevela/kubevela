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

package appfile

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"cuelang.org/go/cue"
	cueerrors "cuelang.org/go/cue/errors"
	"cuelang.org/go/cue/parser"
	"cuelang.org/go/cue/token"
	"github.com/kubevela/pkg/cue/cuex"
	"github.com/kubevela/workflow/pkg/cue/process"
	"k8s.io/klog/v2"

	velacuex "github.com/oam-dev/kubevela/pkg/cue/cuex"
	velaprocess "github.com/oam-dev/kubevela/pkg/cue/process"
	"github.com/oam-dev/kubevela/pkg/policy/ctxschema"
)

// readMessage matches CUE's (v0.14) messages for reading a key a struct may
// not have: an optional one, or an undeclared one of a closed struct. Kept
// here, and pinned by TestReadErrorLabels, so a CUE upgrade rewording them
// fails loudly rather than switching the checks off.
var readMessage = regexp.MustCompile(`(cannot reference optional field|undefined field): ("(?:[^"\\]|\\.)*"|[^\s:;,()]+)`)

// readLabel is a key an error says was read without being certain to exist.
type readLabel struct {
	name     string
	optional bool
}

// readErrorLabels returns the keys an error says were read without being
// certain to exist, in order. The whole message is searched: an interpolation
// wraps the error it failed on.
func readErrorLabels(err error) []readLabel {
	if err == nil {
		return nil
	}
	msgs := []string{err.Error()}
	for _, e := range cueerrors.Errors(err) {
		msgs = append(msgs, e.Error())
	}
	var out []readLabel
	seen := map[readLabel]bool{}
	for _, msg := range msgs {
		for _, m := range readMessage.FindAllStringSubmatch(msg, -1) {
			l := readLabel{name: unquoteLabel(m[2]), optional: m[1] == "cannot reference optional field"}
			if !seen[l] {
				seen[l] = true
				out = append(out, l)
			}
		}
	}
	return out
}

// ctxPass validates a component and its traits again, with context.custom
// typed as the policies that apply declare it, after the ordinary validation
// has passed. A failure is put in terms of the key: one used against its
// type, one optional and read without a guard, one no policy declares.
type ctxPass struct {
	wl      *Component
	ctxData velaprocess.ContextData
	view    *ctxschema.View
	params  map[string]any
	traits  []*Trait
	// chained marks a definition extending another: the files its ancestors
	// are evaluated in cannot take the view's imports.
	chained bool
}

func (p *ctxPass) run() error {
	pCtx, err := p.eval(p.view, 0)
	if err != nil {
		return p.explain(-1, err)
	}
	if err := p.checkReads(pCtx, -1); err != nil {
		return err
	}
	for i, tr := range p.traits {
		if err := p.evalTrait(pCtx, tr, p.imports(p.view)); err != nil {
			return p.explain(i, err)
		}
		if err := p.checkReads(pCtx, i); err != nil {
			return err
		}
	}
	return nil
}

func (p *ctxPass) imports(v *ctxschema.View) string {
	if p.chained {
		return ""
	}
	imports, _ := v.Source()
	return imports
}

func (p *ctxPass) context(v *ctxschema.View) string {
	if p.chained {
		return v.SourceWithoutImports()
	}
	_, ctx := v.Source()
	return ctx
}

// eval evaluates the component, then its first traits, against a view.
func (p *ctxPass) eval(v *ctxschema.View, traits int) (process.Context, error) {
	tmpl, params := p.wl.FullTemplate.TemplateStr, p.wl.Params
	p.wl.FullTemplate.TemplateStr = withImports(tmpl, p.imports(v))
	p.wl.Params = p.params
	defer func() { p.wl.FullTemplate.TemplateStr, p.wl.Params = tmpl, params }()

	pCtx, err := newValidationProcessContext(p.wl, p.ctxData, p.context(v))
	if err != nil {
		return nil, err
	}
	for _, tr := range p.traits[:traits] {
		if err := p.evalTrait(pCtx, tr, p.imports(v)); err != nil {
			return nil, err
		}
	}
	return pCtx, nil
}

func (p *ctxPass) evalTrait(pCtx process.Context, tr *Trait, imports string) error {
	tmpl := tr.Template
	tr.Template = withImports(tmpl, imports)
	defer func() { tr.Template = tmpl }()
	pCtx.PushData(velaprocess.ContextTraitType, tr.Name)
	return tr.EvalContext(pCtx)
}

// owner names the component or trait a stage evaluates, and the templates
// whose reads it judges.
func (p *ctxPass) owner(stage int) (string, []string) {
	if stage >= 0 {
		tr := p.traits[stage]
		return fmt.Sprintf("trait %q", tr.Name), []string{tr.Template}
	}
	templates := []string{p.wl.FullTemplate.TemplateStr}
	for _, a := range p.wl.FullTemplate.Ancestors {
		templates = append(templates, a.Template)
	}
	return fmt.Sprintf("component %q", p.wl.Name), templates
}

// explain puts an evaluation failure in terms of the key that caused it. The
// ordinary validation passed, so the view is what fails: the key is found by
// leaving each one read unchecked in turn. A key read but not declared does
// not fail evaluation; checkReads reports it.
func (p *ctxPass) explain(stage int, err error) error {
	owner, templates := p.owner(stage)
	for _, key := range readTopKeys(contextCustomPaths(templates...)) {
		if !p.view.Declares(key) {
			continue
		}
		if _, err2 := p.eval(p.view.Loosen(key), stage+1); err2 == nil {
			policy, _ := p.view.Optional(key)
			return fmt.Errorf("%s: uses context.custom.%s against its type in policy %q's schema.ctx: %w", owner, key, policy, err)
		}
	}
	// The older evaluator (CUE_EXPERIMENT=evalv3=0, which the chart sets by
	// default) fails evaluation on a read of a key a closed view lacks, where
	// the newer one leaves it incomplete for checkReads to report.
	ctx := p.ctxData.Ctx
	if ctx == nil {
		ctx = context.Background()
	}
	if problems := p.readProblems(ctx, stage, p.context(p.view)); len(problems) > 0 {
		return fmt.Errorf("%s: %s", owner, strings.Join(problems, "; "))
	}
	return fmt.Errorf("%s: does not match context.custom as the applicable policies declare it: %w", owner, err)
}

// checkReads finds the keys a stage reads that may be absent without a guard
// or a default: an optional key, or, when the view is complete, one no policy
// declares. Evaluation alone admits such a read, and the engine prunes what is
// not concrete from the output it keeps, so the stage's own template is
// compiled again, providers unresolved, and its results required concrete.
func (p *ctxPass) checkReads(pCtx process.Context, stage int) error {
	owner, templates := p.owner(stage)
	if len(contextCustomPaths(templates...)) == 0 {
		return nil
	}
	// Evaluation passed, so a failure to rebuild the file here is this check's
	// own: it cannot tell, and the render still refuses what it cannot read.
	contextFile, err := pCtx.BaseContextFile()
	if err != nil {
		klog.V(4).Infof("%s: skipping the context.custom read check: %v", owner, err)
		return nil //nolint:nilerr // the check cannot tell; see above
	}
	problems := p.readProblems(pCtx.GetCtx(), stage, contextFile)
	if len(problems) == 0 {
		return nil
	}
	return fmt.Errorf("%s: %s", owner, strings.Join(problems, "; "))
}

// readProblems compiles a stage's own template with its parameters and
// contextFile, providers unresolved, and describes the reads its results, or
// its compilation, fail on. They come out sorted.
func (p *ctxPass) readProblems(ctx context.Context, stage int, contextFile string) []string {
	tmpl, params, fields := p.wl.FullTemplate.TemplateStr, p.params, []string{"output", "outputs"}
	if stage >= 0 {
		tr := p.traits[stage]
		tmpl, params, fields = tr.Template, tr.Params, []string{"patch", "outputs"}
	}
	paramJSON, err := json.Marshal(params)
	if err != nil || params == nil {
		paramJSON = []byte("{}")
	}
	src := strings.Join([]string{
		renderTemplate(withImports(tmpl, p.imports(p.view))),
		velaprocess.ParameterFieldName + ": " + string(paramJSON),
		contextFile,
	}, "\n")
	reads := contextCustomReadsAt(src)
	seen := map[string]bool{}
	val, err := velacuex.WorkloadCompiler.Get().CompileStringWithOptions(ctx, src, cuex.DisableResolveProviderFunctions{})
	if err != nil {
		problems := p.unguardedReads(reads, err, seen)
		sort.Strings(problems)
		return problems
	}
	var problems []string
	for _, f := range fields {
		v := val.LookupPath(cue.ParsePath(f))
		if !v.Exists() {
			continue
		}
		problems = append(problems, p.unguardedReads(reads, v.Validate(cue.Concrete(true)), seen)...)
	}
	sort.Strings(problems)
	return problems
}

// unguardedReads describes the reads that err says may be absent, skipping and
// then recording the ones in seen. An error names a read by the position CUE
// reports it at, or at an input to it (an interpolation reports the key it
// failed on as an input), never by its key's name alone.
func (p *ctxPass) unguardedReads(reads map[srcPos][]string, err error, seen map[string]bool) []string {
	var problems []string
	for _, e := range cueerrors.Errors(err) {
		labels := readErrorLabels(e)
		for _, pos := range append([]token.Pos{e.Position()}, e.InputPositions()...) {
			path := reads[srcPos{pos.Line(), pos.Column()}]
			if path == nil || seen[strings.Join(path, ".")] || !namesLabel(labels, path[len(path)-1]) {
				continue
			}
			if msg := p.unguarded(path); msg != "" {
				seen[strings.Join(path, ".")] = true
				problems = append(problems, msg)
			}
		}
	}
	return problems
}

func namesLabel(labels []readLabel, name string) bool {
	for _, l := range labels {
		if l.name == name {
			return true
		}
	}
	return false
}

// unguarded describes a read of a path that may be absent, or returns "" when
// the path is certain to be set or the view cannot tell.
func (p *ctxPass) unguarded(path []string) string {
	ref := "context.custom." + strings.Join(path, ".")
	if policy, optional := p.view.PathOptional(path); optional {
		return fmt.Sprintf("%s is optional in policy %q's schema.ctx; guard it (if %s != _|_ {...}) or give it a default", ref, policy, ref)
	}
	if p.view.Complete && !p.view.Declares(path[0]) {
		return fmt.Sprintf("reads %s, but no applicable policy declares it in schema.ctx; guard it (if %s != _|_ {...}) or give it a default", ref, ref)
	}
	return ""
}

func readTopKeys(paths [][]string) []string {
	seen := map[string]bool{}
	var keys []string
	for _, path := range paths {
		if !seen[path[0]] {
			seen[path[0]] = true
			keys = append(keys, path[0])
		}
	}
	return keys
}

func unquoteLabel(label string) string {
	if len(label) >= 2 && label[0] == '"' && label[len(label)-1] == '"' {
		return label[1 : len(label)-1]
	}
	return label
}

// withImports puts imports at the head of a template, after any package
// clause, where CUE requires them.
func withImports(template, imports string) string {
	if imports == "" {
		return template
	}
	f, err := parser.ParseFile("", template, parser.PackageClauseOnly)
	if err != nil || f.PackageName() == "" {
		return imports + template
	}
	end := f.Decls[0].End().Offset()
	return template[:end] + "\n" + imports + template[end:]
}

// policyContextFile adds the view's context.custom declaration to the context
// file every template of a component is evaluated with.
type policyContextFile struct {
	process.Context
	extra string
}

func (c policyContextFile) BaseContextFile() (string, error) {
	base, err := c.Context.BaseContextFile()
	if err != nil {
		return "", err
	}
	return base + "\n" + c.extra, nil
}

// ReadsPolicyContext reports whether a template of any component, its
// ancestors or its traits reads a key of context.custom: if none does, there
// is nothing to judge against the policies' schemas.
func ReadsPolicyContext(af *Appfile) bool {
	for _, wl := range af.ParsedComponents {
		if wl.FullTemplate == nil {
			continue
		}
		templates := []string{wl.FullTemplate.TemplateStr}
		for _, a := range wl.FullTemplate.Ancestors {
			templates = append(templates, a.Template)
		}
		for _, tr := range wl.Traits {
			templates = append(templates, tr.Template)
		}
		if len(contextCustomPaths(templates...)) > 0 {
			return true
		}
	}
	return false
}

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

package cuetest

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"

	"cuelang.org/go/cue"
	"github.com/kubevela/pkg/cue/cuex"
	ginkgotypes "github.com/onsi/ginkgo/v2/types"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/addon"
	"github.com/oam-dev/kubevela/pkg/appfile"
	cuedefinition "github.com/oam-dev/kubevela/pkg/cue/definition"
	"github.com/oam-dev/kubevela/pkg/cue/definition/health"
	"github.com/oam-dev/kubevela/pkg/definition"
	"github.com/oam-dev/kubevela/pkg/definition/propexpr"
	"github.com/oam-dev/kubevela/pkg/sources"
	"github.com/oam-dev/kubevela/pkg/utils"
	"github.com/oam-dev/kubevela/pkg/workflow/providers"
)

// schema is the "vela/test" package test files import.
//
//go:embed schema.cue
var schema string

// Test is the "vela/test" function a case is declared with.
type Test string

const (
	// TestComponentRender is test.#ComponentRender.
	TestComponentRender Test = "component-render"
	// TestTraitRender is test.#TraitRender.
	TestTraitRender Test = "trait-render"
	// TestComponentStatus is test.#ComponentStatus.
	TestComponentStatus Test = "component-status"
	// TestTraitStatus is test.#TraitStatus.
	TestTraitStatus Test = "trait-status"
	// TestAddonRender is test.#AddonRender.
	TestAddonRender Test = "addon-render"
	// TestWorkflowStepExec is test.#WorkflowStepExec.
	TestWorkflowStepExec Test = "workflowstep-exec"
	// TestSourceExec is test.#SourceExec.
	TestSourceExec Test = "source-exec"
	// TestPolicyRender is test.#PolicyRender.
	TestPolicyRender Test = "policy-render"
	// TestApplicationPolicyRender is test.#ApplicationPolicyRender.
	TestApplicationPolicyRender Test = "application-policy-render"
)

var tests = map[Test]struct {
	function string
	kind     Kind
	// mode is what the test does to the definition, and a label on its cases.
	mode string
}{
	TestComponentRender:         {"#ComponentRender", KindComponent, "render"},
	TestTraitRender:             {"#TraitRender", KindTrait, "render"},
	TestComponentStatus:         {"#ComponentStatus", KindComponent, "status"},
	TestTraitStatus:             {"#TraitStatus", KindTrait, "status"},
	TestAddonRender:             {"#AddonRender", KindAddon, "render"},
	TestWorkflowStepExec:        {"#WorkflowStepExec", KindWorkflowStep, "exec"},
	TestSourceExec:              {"#SourceExec", KindSource, "exec"},
	TestPolicyRender:            {"#PolicyRender", KindPolicy, "render"},
	TestApplicationPolicyRender: {"#ApplicationPolicyRender", KindApplicationPolicy, "render"},
}

// labelAttr is the attribute that labels a case, a whole file, or every case
// built from a struct that carries it: @label(slow, env:prod).
const labelAttr = "label"

// upgradeAttr marks a known issue: the definition only works once KubeVela's
// CUE upgrader rewrites it. @upgrade(generic-default-guard, reason="...")
const upgradeAttr = "upgrade"

// pendingAttr parks a case, file or struct the same way, with an optional
// reason: @pending(waiting on the gateway fix). Pending cases load but do not run.
const pendingAttr = "pending"

// testCompiler loads test files: vela/test, plus the workflow's providers
// for hooks to import. Nothing is called while loading.
var testCompiler = sync.OnceValues(func() (*cuex.Compiler, error) {
	pkg, err := testPackage()
	if err != nil {
		return nil, fmt.Errorf("building vela/test: %w", err)
	}
	return cuex.NewCompilerWithInternalPackages(append(providers.WorkflowPackages(), pkg)...), nil
})

// Suite is the cases of one test file, and the hooks that run around them.
type Suite struct {
	File  string
	Cases []*Case
	// Before, BeforeEach, AfterEach and After are the file's hooks of each
	// kind, in the order they are declared.
	Before, BeforeEach, AfterEach, After []*Hook

	// created is what @before and @after hooks created, for RunAfter to delete.
	created createdObjects
	// Err is why the file failed to load, in which case it has no cases.
	Err error
}

// Case is one set of inputs and the result expected from them.
type Case struct {
	Name string
	Test Test
	// Path is the path the case was loaded from, as given to Load, and File
	// and Line locate the case in its test file.
	Path     string
	File     string
	Line     int
	Subject  Subject
	Input    Input
	Observed Observed
	// Expect is the case's `expect`, matched against the result.
	Expect cue.Value
	// Labels are the case's @label labels, its file's, and the automatic ones
	// naming its definition kind and test, sorted.
	Labels []string
	// Pending is set by @pending on the case or its file; the case is not run.
	Pending       bool
	PendingReason string
	// Upgrade is set by @upgrade on the case or its file.
	Upgrade *UpgradeMarker
	// Addon is the addon directory an #AddonRender case renders.
	Addon string
	// Resources are the objects a step or source case creates before the
	// definition runs, and CRDs the CRD paths it installs.
	Resources []map[string]any
	CRDs      []string
	// Source is what a #SourceExec case resolves its source with.
	Source *SourceInput

	suite *Suite
	// root is the whole test file, so a case's checks can run through it.
	root cue.Value
}

// Load reads the test files at path, a single *_test.cue file or a directory
// searched recursively. A file that fails to load is returned as a Suite
// with Err set, so one broken file does not hide the rest; the error return
// is for a path with no test files to load.
func Load(path string) ([]*Suite, error) {
	// Definitions are read through CueX's compilers.
	if err := requireOffline(); err != nil {
		return nil, err
	}
	files, err := testFiles(path)
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no *%s files under %s", utils.CUETestFileSuffix, path)
	}
	l := &loader{subjects: map[string]Subject{}}
	var suites []*Suite
	for _, f := range files {
		s, err := l.suite(f)
		if err != nil {
			s = &Suite{File: f, Err: err}
		}
		for _, c := range s.Cases {
			c.Path = path
		}
		suites = append(suites, s)
	}
	return suites, nil
}

type loader struct {
	// subjects caches loaded definitions by file.
	subjects map[string]Subject
}

func testFiles(path string) ([]string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		if !utils.IsCUETestFile(path) {
			return nil, fmt.Errorf("%s is not a *%s file", path, utils.CUETestFileSuffix)
		}
		return []string{path}, nil
	}
	var files []string
	err = filepath.WalkDir(path, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && utils.IsCUETestFile(p) {
			files = append(files, p)
		}
		return err
	})
	return files, err
}

func (l *loader) suite(file string) (*Suite, error) {
	//nolint:gosec // reading the test files the user named is the point
	src, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	compiler, err := testCompiler()
	if err != nil {
		return nil, err
	}
	v, err := compiler.CompileStringWithOptions(context.Background(), string(src), cuex.DisableResolveProviderFunctions{})
	if err != nil {
		return nil, err
	}
	s := &Suite{File: file}
	fileAttrs := v.Attributes(cue.DeclAttr)
	it, err := v.Fields(cue.Hidden(true))
	if err != nil {
		return nil, err
	}
	hooks := map[HookWhen]*[]*Hook{Before: &s.Before, BeforeEach: &s.BeforeEach, AfterEach: &s.AfterEach, After: &s.After}
	exec := false
	for it.Next() {
		if err := nestedHookMark(it.Value()); err != nil {
			return nil, err
		}
		// A hidden field may be a hook, but never a case: hidden fields
		// hold what cases are built from.
		hidden := it.Selector().LabelType() == cue.HiddenLabel
		name := it.Selector().String()
		if !hidden {
			name = it.Selector().Unquoted()
		}
		isCase := !hidden && it.Value().LookupPath(cue.MakePath(cue.Str("$test"))).Exists()
		if isCase && len(hookMarks(it.Value())) > 0 {
			return nil, fmt.Errorf("case %q: a case cannot be a hook; mark a field of provider calls instead", name)
		}
		h, err := loadHook(file, name, it.Value(), v)
		if err != nil {
			return nil, err
		}
		switch {
		case h != nil:
			*hooks[h.When] = append(*hooks[h.When], h)
			continue
		case !isCase:
			continue
		}
		c, err := l.testCase(file, name, it.Value(), fileAttrs)
		if err != nil {
			return nil, fmt.Errorf("case %q: %w", name, err)
		}
		c.suite, c.root = s, v
		exec = exec || tests[c.Test].mode == "exec"
		s.Cases = append(s.Cases, c)
	}
	if len(s.Cases) == 0 {
		return nil, fmt.Errorf(`no cases: declare each with a function from "vela/test"`)
	}
	if len(s.Before)+len(s.BeforeEach)+len(s.AfterEach)+len(s.After) > 0 && !exec {
		return nil, fmt.Errorf("hooks run against the cluster only step and source cases have, and this file has none")
	}
	return s, nil
}

func (l *loader) testCase(file, name string, v cue.Value, fileAttrs []cue.Attribute) (*Case, error) {
	if err := v.Validate(); err != nil {
		return nil, err
	}
	kind, err := v.LookupPath(cue.MakePath(cue.Str("$test"))).String()
	if err != nil {
		return nil, err
	}
	test, ok := tests[Test(kind)]
	if !ok {
		return nil, fmt.Errorf("unknown test %q", kind)
	}
	if Test(kind) == TestAddonRender {
		return l.addonCase(file, name, v, fileAttrs)
	}
	ref, err := v.LookupPath(cue.ParsePath("definition")).String()
	if err != nil {
		return nil, fmt.Errorf("definition: %w", err)
	}
	subject, err := l.subject(file, ref)
	if err != nil {
		return nil, err
	}
	if subject.Kind == KindApplicationPolicy && test.kind == KindPolicy {
		return nil, fmt.Errorf("%q is Application-scoped, so %s cannot test it; use #ApplicationPolicyRender", subject.Name, test.function)
	}
	if subject.Kind == KindPolicy && test.kind == KindApplicationPolicy {
		return nil, fmt.Errorf("%q is a policy definition, so %s cannot test it; use #PolicyRender", subject.Name, test.function)
	}
	if subject.Kind != test.kind {
		return nil, fmt.Errorf("%q is a %s definition, so %s cannot test it", subject.Name, subject.Kind, test.function)
	}

	traits, err := l.attachedTraits(file, v.LookupPath(cue.ParsePath("traits")))
	if err != nil {
		return nil, err
	}
	c := &Case{
		Name:    name,
		Test:    Test(kind),
		File:    file,
		Line:    v.Pos().Line(),
		Subject: subject,
		Expect:  v.LookupPath(cue.ParsePath("expect")),
	}
	caseAttrs := append(v.Attributes(cue.FieldAttr), v.Attributes(cue.DeclAttr)...)
	if c.Upgrade, err = upgradeMarker(caseAttrs, fileAttrs); err != nil {
		return nil, err
	}
	ownLabels := labelArgs(caseAttrs)
	if c.Upgrade != nil {
		ownLabels = append(ownLabels, upgradeAttr)
	}
	if c.Labels, err = caseLabels(c, labelArgs(fileAttrs), ownLabels); err != nil {
		return nil, err
	}
	c.Pending, c.PendingReason = pending(caseAttrs, fileAttrs)
	if err := checkErrorCategories(c.Expect.LookupPath(cue.ParsePath("error"))); err != nil {
		return nil, err
	}
	if err := checkReferencedAttributes(c.Expect); err != nil {
		return nil, err
	}
	if test.kind == KindSource {
		c.Source = &SourceInput{}
		if err := v.LookupPath(cue.ParsePath("consumer")).Decode(&c.Source.Consumer); err != nil {
			return nil, fmt.Errorf("consumer: %w", err)
		}
		ctx := v.LookupPath(cue.ParsePath("context"))
		if err := checkSourceContext(ctx, c.Source.Consumer, subject); err != nil {
			return nil, err
		}
		if ctx.Exists() {
			if err := ctx.Decode(&c.Source.Context); err != nil {
				return nil, fmt.Errorf("context: %w", err)
			}
		}
	} else if err := checkContext(v.LookupPath(cue.ParsePath("context")), test.kind); err != nil {
		return nil, err
	}
	for field, into := range map[string]any{
		"context":   &c.Input.Context,
		"parameter": &c.Input.Parameter,
		"workload":  &c.Input.Workload,
		"artifacts": &c.Input.Artifacts,
		"spec":      &c.Input.Spec,
		"observed":  &c.Observed,
	} {
		if f := v.LookupPath(cue.ParsePath(field)); f.Exists() {
			if err := f.Validate(cue.Concrete(true)); err != nil {
				return nil, fmt.Errorf("%s must be concrete: %w", field, err)
			}
			if err := f.Decode(into); err != nil {
				return nil, fmt.Errorf("%s: %w", field, err)
			}
		}
	}
	c.Input.Traits = traits
	mockSet := map[Test]*providerSet{TestWorkflowStepExec: execProviders, TestSourceExec: sourceProviders}[c.Test]
	if mockSet == nil {
		mockSet = workloadProviders
	}
	if test.mode == "exec" {
		if c.Resources, c.CRDs, err = clusterInputs(file, v); err != nil {
			return nil, err
		}
	}
	if c.Input.Mocks, err = parseMocksFor(v.LookupPath(cue.ParsePath("mocks")), mockSet); err != nil {
		return nil, err
	}
	return c, nil
}

// addonCase loads an #AddonRender case.
func (l *loader) addonCase(file, name string, v cue.Value, fileAttrs []cue.Attribute) (*Case, error) {
	ref, err := v.LookupPath(cue.ParsePath("addon")).String()
	if err != nil {
		return nil, fmt.Errorf("addon: %w", err)
	}
	dir := filepath.Join(filepath.Dir(file), ref)
	//nolint:gosec // the addon directory is the one the test file names
	raw, err := os.ReadFile(filepath.Join(dir, addon.MetadataFileName))
	if err != nil {
		return nil, fmt.Errorf("addon %q: no %s in %s", ref, addon.MetadataFileName, dir)
	}
	var meta struct {
		Name string `json:"name"`
	}
	if err := yaml.Unmarshal(raw, &meta); err != nil {
		return nil, fmt.Errorf("addon %q: %s: %w", ref, addon.MetadataFileName, err)
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	c := &Case{
		Name:    name,
		Test:    TestAddonRender,
		File:    file,
		Line:    v.Pos().Line(),
		Subject: Subject{Kind: KindAddon, Name: or(meta.Name, filepath.Base(abs))},
		Expect:  v.LookupPath(cue.ParsePath("expect")),
		Addon:   abs,
	}
	caseAttrs := append(v.Attributes(cue.FieldAttr), v.Attributes(cue.DeclAttr)...)
	if c.Upgrade, err = upgradeMarker(caseAttrs, fileAttrs); err != nil {
		return nil, err
	}
	ownLabels := labelArgs(caseAttrs)
	if c.Upgrade != nil {
		ownLabels = append(ownLabels, upgradeAttr)
	}
	if c.Labels, err = caseLabels(c, labelArgs(fileAttrs), ownLabels); err != nil {
		return nil, err
	}
	c.Pending, c.PendingReason = pending(caseAttrs, fileAttrs)
	if err := checkErrorCategories(c.Expect.LookupPath(cue.ParsePath("error"))); err != nil {
		return nil, err
	}
	if err := checkReferencedAttributes(c.Expect); err != nil {
		return nil, err
	}
	if p := v.LookupPath(cue.ParsePath("parameter")); p.Exists() {
		if err := p.Validate(cue.Concrete(true)); err != nil {
			return nil, fmt.Errorf("parameter must be concrete: %w", err)
		}
		if err := p.Decode(&c.Input.Parameter); err != nil {
			return nil, fmt.Errorf("parameter: %w", err)
		}
	}
	if v.LookupPath(cue.ParsePath("mocks")).Exists() {
		return nil, fmt.Errorf("mocks: addon templates compile without providers, so there is nothing to mock")
	}
	return c, nil
}

// clusterInputs reads a step or source case's objects and CRD paths.
func clusterInputs(file string, v cue.Value) ([]map[string]any, []string, error) {
	var objs []map[string]any
	if res := v.LookupPath(cue.ParsePath("resources")); res.Exists() {
		if err := res.Validate(cue.Concrete(true)); err != nil {
			return nil, nil, fmt.Errorf("resources must be concrete: %w", err)
		}
		if err := res.Decode(&objs); err != nil {
			return nil, nil, fmt.Errorf("resources: %w", err)
		}
	}
	var refs, crds []string
	if cr := v.LookupPath(cue.ParsePath("crds")); cr.Exists() {
		if err := cr.Decode(&refs); err != nil {
			return nil, nil, fmt.Errorf("crds: %w", err)
		}
	}
	for _, ref := range refs {
		crds = append(crds, filepath.Join(filepath.Dir(file), ref))
	}
	return objs, crds, nil
}

// attachedTraits loads the traits a component case attaches, in order.
func (l *loader) attachedTraits(file string, v cue.Value) ([]TraitInput, error) {
	if !v.Exists() {
		return nil, nil
	}
	var traits []TraitInput
	it, err := v.List()
	if err != nil {
		return nil, fmt.Errorf("traits: %w", err)
	}
	for i := 0; it.Next(); i++ {
		ref, err := it.Value().LookupPath(cue.ParsePath("definition")).String()
		if err != nil {
			return nil, fmt.Errorf("traits[%d].definition: %w", i, err)
		}
		subject, err := l.subject(file, ref)
		if err != nil {
			return nil, fmt.Errorf("traits[%d].%w", i, err)
		}
		if subject.Kind != KindTrait {
			return nil, fmt.Errorf("traits[%d]: %q is a %s definition, not a trait", i, subject.Name, subject.Kind)
		}
		t := TraitInput{Subject: subject}
		if p := it.Value().LookupPath(cue.ParsePath("parameter")); p.Exists() {
			if err := p.Validate(cue.Concrete(true)); err != nil {
				return nil, fmt.Errorf("traits[%d].parameter must be concrete: %w", i, err)
			}
			if err := p.Decode(&t.Parameter); err != nil {
				return nil, fmt.Errorf("traits[%d].parameter: %w", i, err)
			}
		}
		traits = append(traits, t)
	}
	return traits, nil
}

// caseLabels gathers a case's labels, validated by Ginkgo's rules so a label
// means the same to `vela def test` and a Ginkgo suite.
func caseLabels(c *Case, fileLabels, ownLabels []string) ([]string, error) {
	test := tests[c.Test]
	raw := append([]string{string(test.kind), test.mode}, fileLabels...)
	raw = append(raw, ownLabels...)

	seen := map[string]bool{}
	var labels []string
	for _, l := range raw {
		clean, err := ginkgotypes.ValidateAndCleanupLabel(l, ginkgotypes.CodeLocation{FileName: c.File, LineNumber: c.Line})
		if err != nil {
			reason := err.Error()
			var gerr ginkgotypes.GinkgoError
			if errors.As(err, &gerr) {
				reason = gerr.Message
			}
			return nil, fmt.Errorf("invalid label %q: %s", l, reason)
		}
		if !seen[clean] {
			seen[clean] = true
			labels = append(labels, clean)
		}
	}
	sort.Strings(labels)
	return labels, nil
}

// labelArgs returns the arguments of every @label among attrs.
func labelArgs(attrs []cue.Attribute) []string {
	var labels []string
	for _, a := range attrs {
		if a.Name() == labelAttr {
			labels = append(labels, strings.Split(a.Contents(), ",")...)
		}
	}
	return labels
}

// pending reports the first @pending among the attribute lists, most
// specific first, and its reason.
func pending(lists ...[]cue.Attribute) (bool, string) {
	for _, attrs := range lists {
		for _, a := range attrs {
			if a.Name() == pendingAttr {
				return true, strings.TrimSpace(a.Contents())
			}
		}
	}
	return false, ""
}

// upgradeMarker parses the first @upgrade among the attribute lists, most
// specific first.
func upgradeMarker(lists ...[]cue.Attribute) (*UpgradeMarker, error) {
	for _, attrs := range lists {
		for _, a := range attrs {
			if a.Name() != upgradeAttr {
				continue
			}
			m := &UpgradeMarker{}
			for i := 0; i < a.NumArgs(); i++ {
				key, value := a.Arg(i)
				switch {
				case key == "reason":
					m.Reason = value
				case key == "":
				case !isUpgradePass(key):
					return nil, fmt.Errorf("unknown upgrade pass %q in @upgrade", key)
				default:
					m.Passes = append(m.Passes, key)
				}
			}
			return m, nil
		}
	}
	return nil, nil
}

func isUpgradePass(name string) bool {
	for _, p := range upgradePasses {
		if p.name == name {
			return true
		}
	}
	return false
}

// subject loads the definition a case refers to: a path relative to its test
// file, with ".cue" appended when omitted.
func (l *loader) subject(testFile, ref string) (Subject, error) {
	rel := ref
	if !strings.HasSuffix(rel, ".cue") {
		rel += ".cue"
	}
	file := filepath.Join(filepath.Dir(testFile), rel)
	if s, ok := l.subjects[file]; ok {
		return s, nil
	}
	if _, err := os.Stat(file); errors.Is(err, fs.ErrNotExist) {
		return Subject{}, fmt.Errorf("definition %q: %s does not exist", ref, rel)
	}
	s, err := loadSubject(file)
	if err != nil {
		return Subject{}, fmt.Errorf("%s: %w", file, err)
	}
	l.subjects[file] = s
	return s, nil
}

// loadSubject loads a definition through the controller's dry-run template
// loader, so the template and status snippets are the ones it would use.
func loadSubject(file string) (Subject, error) {
	//nolint:gosec // reading the test files the user named is the point
	src, err := os.ReadFile(file)
	if err != nil {
		return Subject{}, err
	}
	def := definition.Definition{Unstructured: unstructured.Unstructured{}}
	if err := def.FromCUEString(string(src), nil); err != nil {
		return Subject{}, err
	}
	name, kind := def.GetName(), Kind(def.GetType())
	if kind == KindPolicy {
		if scope, _, _ := unstructured.NestedString(def.Object, "spec", "scope"); scope == string(v1beta1.ApplicationScope) {
			kind = KindApplicationPolicy
		}
	}
	if kind == KindWorkflowStep || kind == KindSource || kind == KindApplicationPolicy {
		// The dry-run template loader serves components, traits and
		// workload-bearing policies; these are read off the definition as
		// their own engines read them.
		template, _, err := unstructured.NestedString(def.Object, definition.DefinitionTemplateKeys...)
		if err != nil {
			return Subject{}, err
		}
		return Subject{Kind: kind, Name: name, Template: template, Object: &def.Unstructured}, nil
	}
	capType := map[Kind]types.CapType{KindComponent: types.TypeComponentDefinition, KindTrait: types.TypeTrait, KindPolicy: types.TypePolicy}[kind]
	if capType == "" {
		return Subject{}, fmt.Errorf("%s definitions cannot be tested yet", kind)
	}
	load := appfile.DryRunTemplateLoader([]*unstructured.Unstructured{&def.Unstructured})
	tmpl, err := load(context.Background(), nil, name, capType, nil)
	if err != nil {
		return Subject{}, err
	}
	if kind == KindPolicy && !declaresOutput(tmpl.TemplateStr) {
		// Built-in policies such as topology and override are read by the
		// controller, not rendered.
		return Subject{}, fmt.Errorf("%q renders nothing: its template has no output", name)
	}
	return Subject{
		Kind:         kind,
		Name:         name,
		Template:     tmpl.TemplateStr,
		HealthPolicy: tmpl.Health,
		CustomStatus: tmpl.CustomStatus,
		Details:      tmpl.Details,
		Object:       &def.Unstructured,
	}, nil
}

// Run evaluates the case and returns its failures; none means it passed.
func (c *Case) Run() []string {
	if tests[c.Test].mode == "exec" {
		return c.runExec()
	}
	return c.check(c.result())
}

// check matches what the case produced against its expectations.
func (c *Case) check(result map[string]any, err error) []string {
	var failure caseFailure
	if errors.As(err, &failure) {
		return []string{string(failure)}
	}
	wantErr := c.Expect.LookupPath(cue.ParsePath("error"))
	switch {
	case wantErr.Exists() && err == nil:
		return []string{fmt.Sprintf("error: expected an error matching %v, got none", wantErr)}
	case !wantErr.Exists() && err != nil:
		return []string{"error: unexpected error: " + err.Error()}
	}
	if err != nil {
		result["error"] = err.Error()
		if wantErr.IncompleteKind() == cue.StructKind {
			result["error"] = errorResult(err)
		}
	}
	return match(c.Expect, c.Expect.Context().Encode(result), isCheckCall)
}

// caseFailure fails a case with its message as the one failure, for a
// failure that is not a mismatch, such as a context the render could not
// read that the case asks about.
type caseFailure string

func (f caseFailure) Error() string { return string(f) }

// callsResult files calls by import path, then definition, each in the
// order they ran.
func callsResult(calls []Call) map[string]any {
	out := map[string]any{}
	for _, c := range calls {
		byDef, ok := out[c.Path].(map[string]any)
		if !ok {
			byDef = map[string]any{}
			out[c.Path] = byDef
		}
		call := map[string]any{"$params": c.Params}
		if c.Returns != nil {
			call["$returns"] = c.Returns
		}
		// A call is listed under every definition it is a call of.
		for _, def := range append([]string{c.Def}, c.Also...) {
			list, _ := byDef[def].([]any)
			byDef[def] = append(list, call)
		}
	}
	return out
}

func objects(m map[string]map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// checkContext rejects a context field the definition's surface does not
// offer, or one the case determines itself, and type-checks the rest against
// the context registry.
func checkContext(v cue.Value, kind Kind) error {
	if !v.Exists() {
		return nil
	}
	surface := contextSurfaces[kind]
	it, err := v.Fields()
	if err != nil {
		return fmt.Errorf("context: %w", err)
	}
	for it.Next() {
		field := it.Selector().Unquoted()
		if how, derived := derivedContextFields[kind][field]; derived {
			return fmt.Errorf("context.%s is derived: %s", field, how)
		}
		want, offered := surface.FieldValue(field)
		if !offered {
			msg := fmt.Sprintf("context.%s is not offered to %s", field, surface.Plural())
			if elsewhere := propexpr.SurfacesOffering(field); len(elsewhere) > 0 {
				msg += "; it is offered to " + strings.Join(elsewhere, ", ")
			}
			return errors.New(msg)
		}
		// The registry's types live in its own CUE runtime, so the value is
		// carried over as data.
		var got any
		if err := it.Value().Decode(&got); err != nil {
			return fmt.Errorf("context.%s: %w", field, err)
		}
		if err := want.Unify(want.Context().Encode(got)).Validate(cue.Concrete(true)); err != nil {
			return fmt.Errorf("context.%s: %w", field, err)
		}
	}
	return nil
}

// refiningAttributes are the attributes that refine an expected field.
var refiningAttributes = []string{"exact", "not", "contains"}

// checkReferencedAttributes rejects an expected field that refers to a field
// carrying a refining attribute it does not carry itself: Match reads the
// attributes on the expectation's own fields, not on what they refer to.
func checkReferencedAttributes(expect cue.Value) error {
	var walk func(path []cue.Selector, v cue.Value) error
	walk = func(path []cue.Selector, v cue.Value) error {
		if isCheckCall(path) {
			return nil
		}
		for _, ref := range references(v) {
			root, at := ref.ReferencePath()
			target := root.LookupPath(at)
			for _, name := range refiningAttributes {
				if hasAttribute(target, name) && !hasAttribute(v, name) {
					field := cue.MakePath(append([]cue.Selector{cue.Str("expect")}, path...)...)
					return fmt.Errorf("%s refers to %s, whose @%s() applies only where it is written: put @%s() on %s",
						field, at, name, name, field)
				}
			}
		}
		switch v.IncompleteKind() {
		case cue.StructKind:
			for it, _ := v.Fields(cue.Optional(true)); it.Next(); {
				if err := walk(slices.Concat(path, []cue.Selector{cue.Str(it.Selector().Unquoted())}), it.Value()); err != nil {
					return err
				}
			}
		case cue.ListKind:
			for i, elem := range listElems(v) {
				if err := walk(slices.Concat(path, []cue.Selector{cue.Index(i)}), elem); err != nil {
					return err
				}
			}
		default:
		}
		return nil
	}
	return walk(nil, expect)
}

// references are the references v is built from: v itself, or those it
// unifies or offers as alternatives.
func references(v cue.Value) []cue.Value {
	if _, at := v.ReferencePath(); len(at.Selectors()) > 0 {
		return []cue.Value{v}
	}
	op, args := v.Expr()
	if op != cue.AndOp && op != cue.OrOp {
		return nil
	}
	var refs []cue.Value
	for _, a := range args {
		refs = append(refs, references(a)...)
	}
	return refs
}

func hasAttribute(v cue.Value, name string) bool {
	a := v.Attribute(name)
	return a.Err() == nil
}

// errorCategories are the fields of a structured error expectation.
var errorCategories = map[string]bool{"message": true, "user": true, "parameter": true, "template": true, "status": true, "schema": true}

// checkErrorCategories rejects a structured error expectation naming a
// category that does not exist, which would otherwise never match.
func checkErrorCategories(v cue.Value) error {
	if !v.Exists() || v.IncompleteKind() != cue.StructKind {
		return nil
	}
	it, err := v.Fields(cue.Optional(true))
	if err != nil {
		return err
	}
	for it.Next() {
		if name := it.Selector().Unquoted(); !errorCategories[name] {
			names := sortedKeys(errorCategories)
			return fmt.Errorf("expect.error.%s: not an error category; use %s or %s",
				name, strings.Join(names[:len(names)-1], ", "), names[len(names)-1])
		}
	}
	return nil
}

// errorResult is an error as a structured expectation sees it: its message,
// and its messages by category. A category is present only when it has any.
func errorResult(err error) map[string]any {
	out := map[string]any{"message": err.Error()}
	add := func(category string, msgs []string) {
		if len(msgs) > 0 {
			out[category] = msgs
		}
	}
	var verr *cuedefinition.ValidationError
	var serr *StatusError
	var derr *sources.DefinitionError
	switch {
	case errors.As(err, &derr):
		add("user", derr.User)
		add("schema", derr.Schema)
	case errors.As(err, &verr):
		add("user", verr.User)
		add("parameter", verr.Parameter)
		add("template", verr.Template)
	case errors.As(err, &serr):
		add("status", serr.Errs)
	default:
		add("template", []string{err.Error()})
	}
	return out
}

// result is what the case's test produced, in the shape of its `expect`.
func (c *Case) result() (map[string]any, error) {
	if c.Test == TestApplicationPolicyRender {
		rendered, err := RenderApplicationPolicy(c.Subject, c.Input)
		if err != nil {
			return map[string]any{}, err
		}
		return map[string]any{
			"enabled":     rendered.Enabled,
			"output":      rendered.Output,
			"application": rendered.Application,
			"calls":       callsResult(rendered.Calls),
		}, nil
	}
	if c.Test == TestPolicyRender {
		rendered, err := RenderPolicy(c.Subject, c.Input)
		if err != nil {
			return map[string]any{}, err
		}
		if rendered.ContextErr != nil && c.Expect.LookupPath(cue.ParsePath("context")).Exists() {
			return nil, caseFailure("context: " + rendered.ContextErr.Error())
		}
		return map[string]any{
			"output":  rendered.Output,
			"outputs": objects(rendered.Outputs),
			"context": rendered.Context,
			"calls":   callsResult(rendered.Calls),
		}, nil
	}
	if c.Test == TestAddonRender {
		rendered, err := RenderAddon(c.Addon, c.Input.Parameter)
		if err != nil {
			return map[string]any{}, err
		}
		return rendered.result(), nil
	}
	if tests[c.Test].mode != "status" {
		rendered, err := Render(c.Subject, c.Input)
		if err != nil {
			return map[string]any{}, err
		}
		if rendered.ContextErr != nil && c.Expect.LookupPath(cue.ParsePath("context")).Exists() {
			return nil, caseFailure("context: " + rendered.ContextErr.Error())
		}
		if c.Subject.Kind == KindTrait {
			// The component is only the workload under test; the trait's
			// outputs are the case's.
			return map[string]any{
				"output":  rendered.Output,
				"outputs": objects(rendered.Traits[c.Subject.Name].Outputs),
				"context": rendered.Context,
				"calls":   callsResult(rendered.Calls),
			}, nil
		}
		traits := map[string]any{}
		for typ, t := range rendered.Traits {
			traits[typ] = map[string]any{"outputs": objects(t.Outputs)}
		}
		return map[string]any{
			"output":  rendered.Output,
			"outputs": objects(rendered.Outputs),
			"traits":  traits,
			"context": rendered.Context,
			"calls":   callsResult(rendered.Calls),
		}, nil
	}
	report, err := Status(c.Subject, c.Input, c.Observed)
	if report == nil {
		return map[string]any{}, err
	}
	result := statusResult(report.StatusResult)
	traits := map[string]any{}
	for typ, t := range report.Traits {
		traits[typ] = statusResult(t)
	}
	result["traits"] = traits
	return result, err
}

func statusResult(s *health.StatusResult) map[string]any {
	if s == nil {
		return map[string]any{}
	}
	details := map[string]any{}
	for k, v := range s.Details {
		details[k] = v
	}
	return map[string]any{"healthy": s.Healthy, "message": s.Message, "details": details}
}

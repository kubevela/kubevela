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
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/format"
	"cuelang.org/go/cue/parser"
	"cuelang.org/go/cue/token"
	"github.com/kubevela/pkg/cue/cuex"
	cuexruntime "github.com/kubevela/pkg/cue/cuex/runtime"

	velacuex "github.com/oam-dev/kubevela/pkg/cue/cuex"
	"github.com/oam-dev/kubevela/pkg/workflow/providers"
)

// Mock answers calls to one provider function: those whose $params include
// Params, or every call when Params does not exist.
type Mock struct {
	Params  cue.Value
	Returns cue.Value
}

// Mocks holds a case's mocks by import path, then by definition, such as
// "vela/kube" then "#Get". The first mock that matches a call answers it.
type Mocks map[string]map[string][]Mock

// Call is one provider call a render or step made: its $params, and what
// answered it returned.
type Call struct {
	Path string
	// Def is the most specific definition the call is of, and Also the
	// others it satisfies: an http.#Get call is a #Get, and also a #Do.
	Def     string
	Also    []string
	Params  any
	Returns any
}

// recorder collects provider calls in the order they ran.
type recorder struct {
	mu    sync.Mutex
	calls []Call
	// observe, when set, is told of each call run for real, before it runs,
	// and the function it returns after.
	observe func(call cue.Value) (after func())
}

// add records c; a nil recorder records nothing.
func (r *recorder) add(c Call) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, c)
}

// provider is a package whose functions tests mock, with the #do each of its
// definitions dispatches to.
type provider struct {
	pkg  cuexruntime.Package
	defs map[string]string
	// narrow holds, for a definition derived from another (#Get: #Do &
	// {$params: method: "GET"}), what it adds: what makes a call to its #do
	// one of its own.
	narrow map[string]cue.Value
}

// providerSet is a compiler's providers as tests see them: which can be
// mocked, and what a call no mock answers does.
type providerSet struct {
	packages func() []cuexruntime.Package
	// unmockable are the packages that always run for real, with why.
	unmockable map[string]string
	// unmatched is what a call no mock answers does: nil runs it for real.
	unmatched func(path, def, do string, params []byte) error

	once     sync.Once
	mockable map[string]provider
	// recorded are the unmockable packages, whose calls are recorded.
	recorded map[string]provider
	// pkgs are the packages, built once, in their order.
	pkgs    []cuexruntime.Package
	defsErr error
}

func (s *providerSet) providers() (map[string]provider, error) {
	s.once.Do(func() {
		s.mockable, s.recorded = map[string]provider{}, map[string]provider{}
		s.pkgs = s.packages()
		for _, p := range s.pkgs {
			defs, narrow, err := providerDefinitions(p)
			if err != nil {
				s.defsErr = fmt.Errorf("%s: %w", p.GetPath(), err)
				return
			}
			into := s.mockable
			if _, ok := s.unmockable[p.GetPath()]; ok {
				into = s.recorded
			}
			into[p.GetPath()] = provider{pkg: p, defs: defs, narrow: narrow}
		}
	})
	return s.mockable, s.defsErr
}

// workloadProviders are a render's. Every side-effecting function is mocked,
// so an unmatched call fails rather than reaching a cluster or the network.
var workloadProviders = &providerSet{
	packages: velacuex.WorkloadPackages,
	unmockable: map[string]string{
		"vela/base64": "has no side effects",
		"vela/cue":    "has no side effects",
	},
	unmatched: func(path, def, _ string, params []byte) error {
		return fmt.Errorf("unmocked call %s.%s with $params %s", path, def, params)
	},
}

// execProviders are a step's when it is run. What stays inside the cluster
// runs for real, against the test cluster; what reaches outside it, or needs
// the Application a step test does not have, must be mocked.
var execProviders = &providerSet{
	packages: providers.WorkflowPackages,
	unmockable: map[string]string{
		"vela/builtin": "drives the step's phase",
	},
	unmatched: func(path, def, do string, params []byte) error {
		switch {
		case calls(externalCalls, path, do):
			return fmt.Errorf("unmocked call %s.%s with $params %s: it reaches outside the cluster, so a step test must mock it", path, def, params)
		case calls(applicationCalls, path, do):
			return fmt.Errorf("%s.%s needs an Application, which a step test does not have: mock it", path, def)
		}
		return nil
	},
}

// externalCalls are the workflow provider functions that reach outside the
// cluster, by import path then #do.
var externalCalls = map[string][]string{
	"vela/http":     {"do"},
	"vela/email":    {"send"},
	"vela/metrics":  {"promCheck"},
	"vela/helm":     {"render"},
	"vela/registry": {"read-file"},
	"vela/addon":    {"render"},
	"vela/op":       {"do", "send", "promCheck"},
}

// applicationCalls are the workflow provider functions that call into the
// Application being reconciled, which the controller injects and a step test
// does not have: run for real, they would dereference nothing.
var applicationCalls = map[string][]string{
	"vela/oam":          {"component-apply", "component-render", "load", "load-comps-in-order", "load-policies"},
	"vela/multicluster": {"deploy", "get-placements-from-topology-policies"},
	"vela/terraform":    {"load-terraform-components", "get-connection-status"},
	"vela/op": {"component-apply", "component-render", "load", "load-comps-in-order", "load-policies",
		"deploy", "load-terraform-components", "patch-application", "get-placements-from-topology-policies",
		"make-placement-decisions", "get-connection-status"},
}

func calls(set map[string][]string, path, do string) bool {
	for _, d := range set[path] {
		if d == do {
			return true
		}
	}
	return false
}

// providerDefinitions maps each definition in a package's CUE to its #do,
// and each derived from another, as #Get: #Do & {...} is, to what it adds.
func providerDefinitions(p cuexruntime.Package) (map[string]string, map[string]cue.Value, error) {
	defs := map[string]string{}
	type derived struct {
		base   string
		params cue.Value
	}
	pending := map[string]derived{}
	cctx := cuecontext.New()
	for _, src := range p.GetTemplates() {
		f, err := parser.ParseFile(p.GetPath(), src)
		if err != nil {
			return nil, nil, err
		}
		for _, decl := range f.Decls {
			field, ok := decl.(*ast.Field)
			if !ok {
				continue
			}
			name, _, _ := ast.LabelName(field.Label)
			switch value := field.Value.(type) {
			case *ast.StructLit:
				if do, ok := structDo(value); ok {
					defs[name] = do
				}
			case *ast.BinaryExpr:
				base, ok := value.X.(*ast.Ident)
				extra, isStruct := value.Y.(*ast.StructLit)
				if value.Op != token.AND || !ok || !isStruct {
					continue
				}
				src, err := format.Node(extra)
				if err != nil {
					return nil, nil, fmt.Errorf("%s: %w", name, err)
				}
				pending[name] = derived{base: base.Name, params: cctx.CompileBytes(src)}
			}
		}
	}
	narrow := map[string]cue.Value{}
	// A definition may derive from one that is itself derived.
	for progress := true; progress; {
		progress = false
		for name, d := range pending {
			do, ok := defs[d.base]
			if !ok {
				continue
			}
			params := d.params
			if base, ok := narrow[d.base]; ok {
				params = base.Unify(params)
			}
			defs[name], narrow[name] = do, params
			delete(pending, name)
			progress = true
		}
	}
	return defs, narrow, nil
}

// structDo is the #do a definition's struct declares.
func structDo(body *ast.StructLit) (string, bool) {
	for _, elt := range body.Elts {
		inner, ok := elt.(*ast.Field)
		if !ok {
			continue
		}
		if label, _, _ := ast.LabelName(inner.Label); label != "#do" {
			continue
		}
		if lit, ok := inner.Value.(*ast.BasicLit); ok {
			return strings.Trim(lit.Value, `"`), true
		}
	}
	return "", false
}

// wrap returns the set's packages with every mockable function answering
// from mocks first, each keeping its package's CUE so calls are checked
// against the real signatures, and every call recorded.
func (s *providerSet) wrap(mocks Mocks, rec *recorder) ([]cuexruntime.Package, error) {
	mockable, err := s.providers()
	if err != nil {
		return nil, err
	}
	var pkgs []cuexruntime.Package
	for _, p := range s.pkgs {
		prov, ok := mockable[p.GetPath()]
		unmatched := s.unmatched
		if !ok {
			// An unmockable package still has its calls recorded, run for real.
			if prov, ok = s.recorded[p.GetPath()]; !ok {
				pkgs = append(pkgs, p)
				continue
			}
			unmatched = func(string, string, string, []byte) error { return nil }
		}
		views := map[string][]mockView{}
		for def, do := range prov.defs {
			views[do] = append(views[do], mockView{def: def, narrow: prov.narrow[def], mocks: mocks[p.GetPath()][def]})
		}
		fns := map[string]cuexruntime.ProviderFn{}
		for do, vs := range views {
			// A derived definition's mocks are more specific, so they answer first.
			sort.Slice(vs, func(i, j int) bool {
				if vs[i].narrow.Exists() != vs[j].narrow.Exists() {
					return vs[i].narrow.Exists()
				}
				return vs[i].def < vs[j].def
			})
			fns[do] = mockFn{
				path: p.GetPath(), do: do, views: vs, rec: rec,
				real: p.GetProviderFn(do), unmatched: unmatched,
			}
		}
		pkg, err := cuexruntime.NewInternalPackage(p.GetName(), strings.Join(p.GetTemplates(), "\n"), fns)
		if err != nil {
			return nil, fmt.Errorf("mocking %s: %w", p.GetPath(), err)
		}
		pkgs = append(pkgs, pkg)
	}
	return pkgs, nil
}

// requireOffline refuses to read a compiler singleton that, read first with
// external packages on, loads them from whatever cluster the kubeconfig
// names, or exits the process without one. The switch is process-wide, so it
// is the entry point's to turn off, not this package's.
func requireOffline() error {
	if cuex.EnableExternalPackageForDefaultCompiler {
		return errors.New("rendering needs the process's CueX compilers offline: " +
			"set cuex.EnableExternalPackageForDefaultCompiler = false (github.com/kubevela/pkg/cue/cuex) " +
			"in the test's entry point, such as TestMain or the Ginkgo suite, before any test runs")
	}
	return nil
}

// useMocks puts a compiler into WorkloadCompiler whose side-effecting
// packages answer from mocks, and returns a function putting the real one
// back, and the recorder the calls go to.
func useMocks(mocks Mocks) (restore func(), rec *recorder, err error) {
	rec = &recorder{}
	pkgs, err := workloadProviders.wrap(mocks, rec)
	if err != nil {
		return nil, nil, err
	}
	if err := requireOffline(); err != nil {
		return nil, nil, err
	}
	real := velacuex.WorkloadCompiler.Get()
	velacuex.WorkloadCompiler.Set(cuex.NewCompilerWithInternalPackages(pkgs...))
	return func() { velacuex.WorkloadCompiler.Set(real) }, rec, nil
}

// useExecProviders is a workflow compiler for running a step: mocks answer
// first, and what they do not answer runs for real unless it reaches outside
// the cluster or needs an Application. The engine takes the compiler
// directly, so nothing global is replaced.
func useExecProviders(mocks Mocks) (*cuex.Compiler, *recorder, error) {
	rec := &recorder{}
	pkgs, err := execProviders.wrap(mocks, rec)
	if err != nil {
		return nil, nil, err
	}
	return cuex.NewCompilerWithInternalPackages(pkgs...), rec, nil
}

// mockFn answers every call to one #do, which the definitions sharing it
// view differently: a call is each one's whose narrowing its $params satisfy.
type mockFn struct {
	path, do  string
	views     []mockView
	rec       *recorder
	real      cuexruntime.ProviderFn
	unmatched func(path, def, do string, params []byte) error
}

// mockView is one definition's view of calls to its #do: which are its own,
// and the mocks answering them.
type mockView struct {
	def    string
	narrow cue.Value
	mocks  []Mock
}

// owns reports whether a call is one of the definition's: whether it has
// what the definition adds, in $params or beside it.
func (v mockView) owns(call cue.Value, params any) bool {
	if !v.narrow.Exists() {
		return true
	}
	actual := map[string]any{}
	it, err := v.narrow.Fields()
	if err != nil {
		return false
	}
	for it.Next() {
		field := it.Selector().Unquoted()
		if field == "$params" {
			actual[field] = params
			continue
		}
		var got any
		if f := call.LookupPath(cue.MakePath(cue.Str(field))); f.Exists() && f.Decode(&got) == nil {
			actual[field] = got
		}
	}
	return len(Match(v.narrow, v.narrow.Context().Encode(actual))) == 0
}

var (
	paramsPath  = cue.MakePath(cue.Str("$params"))
	returnsPath = cue.MakePath(cue.Str("$returns"))
)

// legacyProviders are the providers of vela/op and vela/ql's legacy
// functions, which take their parameters, and give their results, at the top
// level rather than under $params and $returns.
var legacyProviders = map[string]bool{"op": true, "ql": true}

// isLegacy reports whether call is of a legacy function, as its #provider
// declares.
func isLegacy(call cue.Value) bool {
	provider, err := call.LookupPath(cue.ParsePath(providerKey)).String()
	return err == nil && legacyProviders[provider]
}

func (f mockFn) Call(ctx context.Context, value cue.Value) (cue.Value, error) {
	var params any
	legacy := isLegacy(value)
	if legacy {
		// A legacy provider marshals the whole call first, so a call whose
		// inputs are broken fails with that, mocked or not.
		if _, err := value.MarshalJSON(); err != nil {
			return value, err
		}
		params = legacyParams(value)
	} else if p := value.LookupPath(paramsPath); p.Exists() {
		if err := p.Decode(&params); err != nil {
			return value, fmt.Errorf("%s.%s: decoding $params: %w", f.path, f.views[0].def, err)
		}
	}
	var owners []mockView
	for _, v := range f.views {
		if v.owns(value, params) {
			owners = append(owners, v)
		}
	}
	record := func(returns any) {
		call := Call{Path: f.path, Def: owners[0].def, Params: params, Returns: returns}
		for _, v := range owners[1:] {
			call.Also = append(call.Also, v.def)
		}
		f.rec.add(call)
	}
	for _, v := range owners {
		for _, m := range v.mocks {
			// The call's parameters are carried into the mock's runtime as data.
			if m.Params.Exists() && len(Match(m.Params, m.Params.Context().Encode(params))) > 0 {
				continue
			}
			if !m.Returns.Exists() {
				record(nil)
				return value, nil
			}
			var returns any
			if err := m.Returns.Decode(&returns); err != nil {
				return value, fmt.Errorf("%s.%s: decoding the mock's $returns: %w", f.path, v.def, err)
			}
			at := returnsPath
			if legacy {
				at = cue.MakePath()
			}
			filled := value.FillPath(at, returns)
			if err := filled.LookupPath(at).Validate(); err != nil {
				return value, fmt.Errorf("%s.%s: the mock's $returns does not fit its signature: %w", f.path, v.def, err)
			}
			record(returns)
			return filled, nil
		}
	}
	shown, _ := json.Marshal(params)
	if err := f.unmatched(f.path, owners[0].def, f.do, shown); err != nil {
		return value, err
	}
	var after func()
	if f.rec != nil && f.rec.observe != nil {
		after = f.rec.observe(value)
	}
	out, err := f.real.Call(ctx, value)
	if after != nil {
		after()
	}
	var returns any
	switch r := out.LookupPath(returnsPath); {
	case err != nil:
	case legacy:
		if results := legacyReturns(out, params); results != nil {
			returns = results
		}
	case r.Exists():
		_ = r.Decode(&returns)
	}
	record(returns)
	return out, err
}

// legacyReturns are a legacy call's results: its concrete top-level fields
// that are new, or changed from its parameters; nil when there are none.
func legacyReturns(out cue.Value, params any) map[string]any {
	in, _ := params.(map[string]any)
	returns := map[string]any{}
	for k, v := range legacyParams(out) {
		if was, ok := in[k]; !ok || !reflect.DeepEqual(was, v) {
			returns[k] = v
		}
	}
	if len(returns) == 0 {
		return nil
	}
	return returns
}

// legacyParams are a legacy call's concrete top-level fields, its
// parameters so far as they are known when it is made.
func legacyParams(call cue.Value) map[string]any {
	params := map[string]any{}
	it, err := call.Fields()
	if err != nil {
		return params
	}
	for it.Next() {
		var v any
		if it.Value().Validate(cue.Concrete(true)) == nil && it.Value().Decode(&v) == nil {
			params[it.Selector().Unquoted()] = v
		}
	}
	return params
}

// parseMocksFor reads a case's mocks, rejecting a package or function the
// set cannot mock.
func parseMocksFor(v cue.Value, set *providerSet) (Mocks, error) {
	if !v.Exists() {
		return nil, nil
	}
	mockable, err := set.providers()
	if err != nil {
		return nil, err
	}
	out := Mocks{}
	pkgs, err := v.Fields()
	if err != nil {
		return nil, fmt.Errorf("mocks: %w", err)
	}
	for pkgs.Next() {
		path := pkgs.Selector().Unquoted()
		prov, ok := mockable[path]
		if why, never := set.unmockable[path]; never {
			return nil, fmt.Errorf("mocks.%q: %s %s, so it runs for real", path, path, why)
		}
		if !ok {
			return nil, fmt.Errorf("mocks.%q: not a provider; mockable: %s", path, strings.Join(sortedKeys(mockable), ", "))
		}
		out[path] = map[string][]Mock{}
		defs, err := pkgs.Value().Fields()
		if err != nil {
			return nil, fmt.Errorf("mocks.%q: %w", path, err)
		}
		for defs.Next() {
			def := defs.Selector().Unquoted()
			if _, ok := prov.defs[def]; !ok {
				return nil, fmt.Errorf("mocks.%q.%q: %s has no %s; it has %s", path, def, path, def, strings.Join(sortedKeys(prov.defs), ", "))
			}
			var entries []cue.Value
			if defs.Value().IncompleteKind() == cue.ListKind {
				entries = listElems(defs.Value())
			} else {
				entries = []cue.Value{defs.Value()}
			}
			for _, e := range entries {
				out[path][def] = append(out[path][def], Mock{Params: e.LookupPath(paramsPath), Returns: e.LookupPath(returnsPath)})
			}
		}
	}
	return out, nil
}

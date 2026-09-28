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
	"errors"
	"fmt"
	"path/filepath"
	"sync"

	"cuelang.org/go/cue"
	"github.com/kubevela/pkg/cue/cuex"
	cuexruntime "github.com/kubevela/pkg/cue/cuex/runtime"
	cueutil "github.com/kubevela/pkg/cue/util"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/types"
)

// HookWhen is when a hook runs, and the attribute that marks it.
type HookWhen string

// The hook attributes, in the order they run around a case.
const (
	Before     HookWhen = "before"
	BeforeEach HookWhen = "beforeEach"
	AfterEach  HookWhen = "afterEach"
	After      HookWhen = "after"
)

var hookWhens = []HookWhen{Before, BeforeEach, AfterEach, After}

// Hook is a field of a test file marked to run around its cases: every
// provider function called in it, in order, against the test cluster.
type Hook struct {
	Name string
	When HookWhen
	File string
	Line int

	// root is the whole file, so a call can use an earlier one's $returns.
	root cue.Value
	path cue.Path
}

// hookNamespace is where a hook's objects go when they name no namespace:
// the case's around each case, and default around the file.
const hookNamespace = "default"

// callEnv is where a hook's or a check's provider calls run, and what
// vela/test's functions act on.
type callEnv struct {
	cluster   *TestCluster
	namespace string
	dir       string
	created   *createdObjects
}

// createdObjects are the objects created in one scope, a case or a file, to
// delete newest first when it ends. Namespaces are left: envtest runs no
// namespace controller, so a deleted one stays Terminating and refuses new
// objects.
type createdObjects struct {
	objs []*unstructured.Unstructured
}

func (c *createdObjects) add(obj *unstructured.Unstructured) {
	if c == nil || (obj.GetAPIVersion() == "v1" && obj.GetKind() == "Namespace") {
		return
	}
	for _, o := range c.objs {
		if o.GroupVersionKind() == obj.GroupVersionKind() && o.GetNamespace() == obj.GetNamespace() && o.GetName() == obj.GetName() {
			return
		}
	}
	ref := &unstructured.Unstructured{}
	ref.SetGroupVersionKind(obj.GroupVersionKind())
	ref.SetNamespace(obj.GetNamespace())
	ref.SetName(obj.GetName())
	c.objs = append(c.objs, ref)
}

// cleanup deletes every object created in the scope, whatever fails.
func (c *createdObjects) cleanup(cl *TestCluster) error {
	var errs []error
	for i := len(c.objs) - 1; i >= 0; i-- {
		o := c.objs[i]
		// Background, as nothing here collects what an orphaning delete leaves.
		err := cl.Client.Delete(context.Background(), o, client.PropagationPolicy(metav1.DeletePropagationBackground))
		if err != nil && !apierrors.IsNotFound(err) {
			errs = append(errs, fmt.Errorf("cleaning up %s %s/%s: %w", o.GetKind(), o.GetNamespace(), o.GetName(), err))
		}
	}
	c.objs = nil
	return errors.Join(errs...)
}

type callEnvKey struct{}

// hookMarks are the hook attributes on a field, or inside its struct.
func hookMarks(v cue.Value) []HookWhen {
	var marks []HookWhen
	attrs := append(v.Attributes(cue.FieldAttr), v.Attributes(cue.DeclAttr)...)
	for _, when := range hookWhens {
		for _, a := range attrs {
			if a.Name() == string(when) {
				marks = append(marks, when)
				break
			}
		}
	}
	return marks
}

// nestedHookMark reports a hook attribute below a top-level field: a hook is
// a whole top-level field, whose calls all run, however deep.
func nestedHookMark(v cue.Value) error {
	var walk func(v cue.Value, top bool) error
	walk = func(v cue.Value, top bool) error {
		if marks := hookMarks(v); !top && len(marks) > 0 {
			return fmt.Errorf("%s: @%s marks a nested field; mark the top-level field instead", v.Path(), marks[0])
		}
		var children []cue.Value
		switch v.IncompleteKind() {
		case cue.StructKind:
			for it, _ := v.Fields(cue.Optional(true), cue.Hidden(true)); it.Next(); {
				children = append(children, it.Value())
			}
		case cue.ListKind:
			children = listElems(v)
		default:
		}
		for _, c := range children {
			if err := walk(c, false); err != nil {
				return err
			}
		}
		return nil
	}
	return walk(v, true)
}

// loadHook reads a field marked as a hook; it is nil for a field without.
func loadHook(file, name string, v cue.Value, root cue.Value) (*Hook, error) {
	marks := hookMarks(v)
	if len(marks) == 0 {
		return nil, nil
	}
	if len(marks) > 1 {
		return nil, fmt.Errorf("hook %q: marked both @%s and @%s", name, marks[0], marks[1])
	}
	if err := v.Validate(); err != nil {
		return nil, fmt.Errorf("hook %q: %w", name, err)
	}
	calls := false
	cueutil.Iterate(v, func(v cue.Value) bool {
		calls = v.LookupPath(cue.ParsePath(doKey)).Exists()
		return calls
	})
	if !calls {
		return nil, fmt.Errorf("hook %q: calls no provider function, such as kube.#Apply or test.#Seed", name)
	}
	return &Hook{Name: name, When: marks[0], File: file, Line: v.Pos().Line(), root: root, path: v.Path()}, nil
}

const (
	doKey       = "#do"
	providerKey = "#provider"
)

// run calls the hook's provider functions for real, in order, with objects
// that name no namespace going into ns. Mocks do not apply: a hook builds
// the platform the case runs on.
func (h *Hook) run(cl *TestCluster, ns string, created *createdObjects) error {
	if err := h.call(cl, ns, created); err != nil {
		return fmt.Errorf("@%s %q: %w", h.When, h.Name, err)
	}
	return nil
}

func (h *Hook) call(cl *TestCluster, ns string, created *createdObjects) error {
	if err := cl.ensureNamespace(ns); err != nil {
		return err
	}
	_, err := resolveCalls(h.root, h.path, &callEnv{cluster: cl, namespace: ns, dir: filepath.Dir(h.File), created: created})
	if err != nil {
		return err
	}
	return nil
}

// callError is a provider call under a path that failed, or could not run.
type callError struct {
	at  cue.Path
	err error
}

func (e *callError) Error() string { return fmt.Sprintf("%s: %v", e.at, e.err) }

func (e *callError) Unwrap() error { return e.err }

// resolveCalls calls every provider function under path in root, for real
// and in order, and returns root with their $returns filled in. Calls go
// through root, not a value looked up from it, so a call can use an earlier
// one's $returns.
func resolveCalls(root cue.Value, path cue.Path, env *callEnv) (cue.Value, error) {
	compiler, err := hookCompiler()
	if err != nil {
		return root, err
	}
	providers := compiler.PackageManager.GetProviders()
	ctx := context.WithValue(env.cluster.runtimeContext(context.Background()), callEnvKey{}, env)
	done := map[string]bool{}
	for {
		var next *cue.Value
		cueutil.Iterate(root.LookupPath(path), func(v cue.Value) bool {
			if fn, _ := v.LookupPath(cue.ParsePath(doKey)).String(); fn != "" && !done[v.Path().String()] {
				next = &v
				return true
			}
			return false
		})
		if next == nil {
			return root, nil
		}
		at := next.Path()
		done[at.String()] = true
		do, _ := next.LookupPath(cue.ParsePath(doKey)).String()
		name, _ := next.LookupPath(cue.ParsePath(providerKey)).String()
		provider, ok := providers[name]
		if !ok {
			return root, &callError{at, fmt.Errorf("no provider %q", name)}
		}
		fn := provider.GetProviderFn(do)
		if fn == nil {
			return root, &callError{at, fmt.Errorf("provider %q has no function %q", name, do)}
		}
		// vela/op's legacy functions take their parameters at the top level.
		if params := next.LookupPath(cue.ParsePath("$params")); params.Exists() {
			if err := params.Validate(cue.Concrete(true)); err != nil {
				return root, &callError{at, fmt.Errorf("$params must be concrete: %w", err)}
			}
		}
		if name == "kube" || name == "op" {
			root = env.fillNamespaces(root, at)
			*next = root.LookupPath(at)
		}
		// vela/test's functions record what they create themselves; for the
		// rest, an object the call names that is there after the call but
		// was not before is the call's.
		created := func() []*unstructured.Unstructured { return nil }
		if name != "test" {
			created = env.cluster.watchCreates(*next, env.namespace)
		}
		out, err := callProvider(ctx, fn, *next, name, do)
		for _, obj := range created() {
			env.created.add(obj)
		}
		if err != nil {
			return root, &callError{at, err}
		}
		root = root.FillPath(at, out)
	}
}

// callProvider calls fn, turning a panic into an error: a provider bug fails
// the case that reached it, not the whole run.
func callProvider(ctx context.Context, fn cuexruntime.ProviderFn, call cue.Value, provider, do string) (out cue.Value, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("provider %s.%s panicked: %v", provider, do, r)
		}
	}()
	return fn.Call(ctx, call)
}

// hookCompiler holds every provider a hook or check may call: the
// workflow's, real but for those that reach outside the cluster or need an
// Application, and vela/test's own. Their calls are not a case's, so nothing
// records them.
var hookCompiler = sync.OnceValues(func() (*cuex.Compiler, error) {
	pkgs, err := execProviders.wrap(nil, nil)
	if err != nil {
		return nil, err
	}
	test, err := testPackage()
	if err != nil {
		return nil, err
	}
	return cuex.NewCompilerWithInternalPackages(append(pkgs, test)...), nil
})

// testPackage is vela/test: the case and hook schema, and the functions
// only a hook can call.
func testPackage() (cuexruntime.Package, error) {
	return cuexruntime.NewInternalPackage("test", schema, map[string]cuexruntime.ProviderFn{
		"seed":         cuexruntime.GenericProviderFn[seedParams, struct{}](seed),
		"install-crds": cuexruntime.GenericProviderFn[installCRDsParams, struct{}](installCRDs),
	})
}

type seedParams struct {
	Params struct {
		Objects []map[string]any `json:"objects"`
	} `json:"$params"`
}

func seed(ctx context.Context, p *seedParams) (*struct{}, error) {
	env, err := callEnvFrom(ctx)
	if err != nil {
		return nil, err
	}
	return &struct{}{}, env.cluster.seed(env.namespace, p.Params.Objects, env.created)
}

type installCRDsParams struct {
	Params struct {
		Paths []string `json:"paths"`
	} `json:"$params"`
}

func installCRDs(ctx context.Context, p *installCRDsParams) (*struct{}, error) {
	env, err := callEnvFrom(ctx)
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, ref := range p.Params.Paths {
		paths = append(paths, filepath.Join(env.dir, ref))
	}
	return &struct{}{}, env.cluster.installCRDs(paths)
}

func callEnvFrom(ctx context.Context) (*callEnv, error) {
	env, ok := ctx.Value(callEnvKey{}).(*callEnv)
	if !ok {
		return nil, fmt.Errorf("vela/test functions run only in a hook or a check")
	}
	return env, nil
}

// runBefore runs hooks in order, stopping at the first to fail.
func runBefore(hooks []*Hook, cl *TestCluster, ns string, created *createdObjects) error {
	for _, h := range hooks {
		if err := h.run(cl, ns, created); err != nil {
			return err
		}
	}
	return nil
}

// runAfter runs every hook, whatever fails, then deletes what the scope
// created.
func runAfter(hooks []*Hook, cl *TestCluster, ns string, created *createdObjects) error {
	var errs []error
	for _, h := range hooks {
		errs = append(errs, h.run(cl, ns, created))
	}
	errs = append(errs, created.cleanup(cl))
	return errors.Join(errs...)
}

// valuePath is where a call names its objects: $params.value, or value for
// vela/op's legacy functions.
func valuePath(call cue.Value) cue.Path {
	if isLegacy(call) {
		return cue.ParsePath("value")
	}
	return cue.ParsePath("$params.value")
}

// fillNamespaces gives each namespaced object the call at names, one or a
// list, the scope's namespace where it names none: an unnamed namespace
// means the same in a kube call as in test.#Seed and expect.resources.
func (env *callEnv) fillNamespaces(root cue.Value, at cue.Path) cue.Value {
	path := cue.MakePath(append(at.Selectors(), valuePath(root.LookupPath(at)).Selectors()...)...)
	value := root.LookupPath(path)
	targets := []cue.Path{path}
	if value.IncompleteKind() == cue.ListKind {
		targets = nil
		for i := range listElems(value) {
			targets = append(targets, cue.MakePath(append(path.Selectors(), cue.Index(i))...))
		}
	}
	for _, p := range targets {
		obj := map[string]any{}
		if err := root.LookupPath(p).Decode(&obj); err != nil {
			continue
		}
		u := &unstructured.Unstructured{Object: obj}
		if u.GetKind() == "" || u.GetNamespace() != "" {
			continue
		}
		if namespaced, err := env.cluster.Client.IsObjectNamespaced(u); err == nil && namespaced {
			root = root.FillPath(cue.MakePath(append(p.Selectors(), cue.Str("metadata"), cue.Str("namespace"))...), env.namespace)
		}
	}
	return root
}

// watchCreates starts watching what call creates, and returns a function
// reporting it once the call has run. For a vela/config create that is the
// Config Secrets new to its namespace, as the Config's template may name the
// Secret; for any other call, the objects it names that were not there
// before, those naming no namespace looked for in ns.
func (c *TestCluster) watchCreates(call cue.Value, ns string) func() []*unstructured.Unstructured {
	if configNS, ok := configCreateNamespace(call); ok {
		before := c.configSecrets(configNS)
		return func() []*unstructured.Unstructured {
			var created []*unstructured.Unstructured
			for _, obj := range c.configSecrets(configNS) {
				if _, existed := before[obj.GetName()]; !existed {
					created = append(created, obj)
				}
			}
			return created
		}
	}
	fresh := c.absent(callObjects(call), ns)
	return func() []*unstructured.Unstructured {
		var created []*unstructured.Unstructured
		for _, obj := range fresh {
			if c.exists(obj) {
				created = append(created, obj)
			}
		}
		return created
	}
}

// configCreateNamespace is the namespace a vela/config create call stores its
// Config in; false for any other call.
func configCreateNamespace(call cue.Value) (string, bool) {
	provider, _ := call.LookupPath(cue.ParsePath(providerKey)).String()
	do, _ := call.LookupPath(cue.ParsePath(doKey)).String()
	if provider != "config" || do != "create" {
		return "", false
	}
	ns, err := call.LookupPath(cue.ParsePath("$params.namespace")).String()
	return ns, err == nil && ns != ""
}

// configSecrets are the Secrets holding Configs in ns, by name.
func (c *TestCluster) configSecrets(ns string) map[string]*unstructured.Unstructured {
	list := &unstructured.UnstructuredList{}
	list.SetAPIVersion("v1")
	list.SetKind("SecretList")
	out := map[string]*unstructured.Unstructured{}
	if err := c.Client.List(context.Background(), list, client.InNamespace(ns),
		client.MatchingLabels{types.LabelConfigCatalog: types.VelaCoreConfig}); err != nil {
		return out
	}
	for i := range list.Items {
		out[list.Items[i].GetName()] = &list.Items[i]
	}
	return out
}

// callObjects are the objects a call names in its value, one or a list.
func callObjects(call cue.Value) []*unstructured.Unstructured {
	var value any
	if err := call.LookupPath(valuePath(call)).Decode(&value); err != nil {
		return nil
	}
	items, ok := value.([]any)
	if !ok {
		items = []any{value}
	}
	var objs []*unstructured.Unstructured
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		obj := &unstructured.Unstructured{Object: m}
		if obj.GetKind() == "" || obj.GetName() == "" {
			continue
		}
		objs = append(objs, obj)
	}
	return objs
}

// absent are the objects that do not exist yet, those naming no namespace
// looked for in ns.
func (c *TestCluster) absent(objs []*unstructured.Unstructured, ns string) []*unstructured.Unstructured {
	var out []*unstructured.Unstructured
	for _, obj := range objs {
		if namespaced, err := c.Client.IsObjectNamespaced(obj); err == nil && namespaced && obj.GetNamespace() == "" {
			obj.SetNamespace(ns)
		}
		if !c.exists(obj) {
			out = append(out, obj)
		}
	}
	return out
}

func (c *TestCluster) exists(obj *unstructured.Unstructured) bool {
	got := &unstructured.Unstructured{}
	got.SetGroupVersionKind(obj.GroupVersionKind())
	return c.Client.Get(context.Background(), client.ObjectKeyFromObject(obj), got) == nil
}

// RunBefore runs the suite's @before hooks, starting the test cluster if
// there are any.
func (s *Suite) RunBefore() error {
	if len(s.Before) == 0 {
		return nil
	}
	cl, err := execCluster(s.File)
	if err != nil {
		return err
	}
	return runBefore(s.Before, cl, hookNamespace, &s.created)
}

// RunAfter runs the suite's @after hooks, then deletes what its @before and
// @after hooks created.
func (s *Suite) RunAfter() error {
	if len(s.After) == 0 && len(s.created.objs) == 0 {
		return nil
	}
	cl, err := execCluster(s.File)
	if err != nil {
		return err
	}
	return runAfter(s.After, cl, hookNamespace, &s.created)
}

// Evaluate runs cases, the suite's that are to run, in order, between its
// @before and @after hooks, and returns their outcomes in the same order.
// As in Ginkgo, a failing @before fails every case, and a failing @after
// fails the last; with no cases, no hook runs.
func (s *Suite) Evaluate(cases []*Case, opts RunOptions) []Outcome {
	if len(cases) == 0 {
		return nil
	}
	outcomes := make([]Outcome, len(cases))
	if err := s.RunBefore(); err != nil {
		for i := range outcomes {
			outcomes[i] = Outcome{Failures: []string{err.Error()}}
		}
	} else {
		for i, c := range cases {
			outcomes[i] = c.Evaluate(opts)
		}
	}
	if err := s.RunAfter(); err != nil {
		last := &outcomes[len(outcomes)-1]
		last.Failures = append(last.Failures, err.Error())
		last.Upgraded = false
	}
	return outcomes
}

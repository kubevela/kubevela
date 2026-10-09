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
	"strings"
	"sync"

	"cuelang.org/go/cue"
	oamv1alpha1 "github.com/kubevela/pkg/apis/oam/v1alpha1"
	"github.com/kubevela/pkg/cue/cuex"
	monitorContext "github.com/kubevela/pkg/monitor/context"
	"github.com/kubevela/pkg/util/singleton"
	wfcontext "github.com/kubevela/workflow/pkg/context"
	"github.com/kubevela/workflow/pkg/tasks/custom"
	wftypes "github.com/kubevela/workflow/pkg/types"

	"github.com/oam-dev/kubevela/pkg/appfile"
	"github.com/oam-dev/kubevela/pkg/cue/process"
	"github.com/oam-dev/kubevela/pkg/oam"
)

// The identity the harness gives a step where a workflow would give its own.
const (
	testStepName = "test-step"
	testStepID   = "test-step-id"
)

// StepRun is what running a workflow step did.
type StepRun struct {
	// Phase, Reason and Message are the step's status, as the engine sets it.
	Phase   string
	Reason  string
	Message string
	// Calls are the provider calls the step made, real or mocked, in order.
	Calls []Call
	// Namespace is where the step ran.
	Namespace string
}

// ExecWorkflowStep runs a step through the workflow engine's own task
// runner, against the shared test cluster: its CRDs installed, its objects
// created, in the case's namespace or a fresh one. near is a path the
// vela-core chart's CRDs are looked for from.
func ExecWorkflowStep(s Subject, in Input, objs []map[string]any, crds []string, near string) (*StepRun, error) {
	cl, ns, err := stepCluster(near, crds, in.Context.Namespace)
	if err != nil {
		return nil, err
	}
	return runStep(cl, ns, s, in, objs, nil)
}

// stepCluster is the shared test cluster with crds installed, and the
// namespace a step runs in: ns, or a fresh one.
func stepCluster(near string, crds []string, ns string) (*TestCluster, string, error) {
	cl, err := execCluster(near)
	if err != nil {
		return nil, "", err
	}
	if len(crds) > 0 {
		if err := cl.installCRDs(crds); err != nil {
			return nil, "", fmt.Errorf("installing crds: %w", err)
		}
	}
	if ns == "" {
		ns, err = cl.NewNamespace()
		return cl, ns, err
	}
	return cl, ns, cl.ensureNamespace(ns)
}

// runStep creates objs in ns, then runs the step there, adding what the
// step creates to created when that is given.
func runStep(cl *TestCluster, ns string, s Subject, in Input, objs []map[string]any, created *createdObjects) (*StepRun, error) {
	if err := cl.Seed(ns, objs); err != nil {
		return nil, err
	}
	// The workflow providers and context reach the cluster through this.
	singleton.KubeClient.Set(cl.Client)
	c := in.Context

	compiler, rec, err := useExecProviders(in.Mocks)
	if err != nil {
		return nil, err
	}
	if created != nil {
		// What the step creates is the case's, and deleted with it. A step's
		// own call naming no namespace is applied into default, as the kube
		// provider defaults it.
		rec.observe = func(call cue.Value) func() {
			done := cl.watchCreates(call, hookNamespace)
			return func() {
				for _, obj := range done() {
					created.add(obj)
				}
			}
		}
	}
	app := application(c)
	app.Namespace = ns
	goCtx := cl.runtimeContext(context.Background())
	if c.Custom != nil {
		goCtx = context.WithValue(goCtx, oam.PolicyAdditionalContextKey, c.Custom)
	}
	data := appfile.WorkflowContextData(goCtx, app, or(c.AppRevision, app.Name+"-v1"))
	data.ClusterVersion = clusterVersion(c.ClusterVersion)
	pCtx := process.NewContext(data)

	loader := custom.NewTaskLoader(func(context.Context, string) (string, error) { return s.Template, nil }, 0, pCtx, compiler)
	generate, err := loader.GetTaskGenerator(goCtx, s.Name)
	if err != nil {
		return nil, err
	}
	props, err := rawJSON(in.Parameter)
	if err != nil {
		return nil, err
	}
	step := oamv1alpha1.WorkflowStep{WorkflowStepBase: oamv1alpha1.WorkflowStepBase{
		Name: or(c.StepName, testStepName), Type: s.Name, Properties: props,
	}}
	runner, err := generate(step, &wftypes.TaskGeneratorOptions{ID: testStepID})
	if err != nil {
		return nil, err
	}
	// The engine keeps a step's retry counts in memory by Application, across
	// its runs; a case is one run of its own, so it starts and ends clean.
	wfcontext.CleanupMemoryStore(app.Name, ns)
	defer wfcontext.CleanupMemoryStore(app.Name, ns)
	wfCtx, err := wfcontext.NewContext(goCtx, ns, app.Name, nil)
	if err != nil {
		return nil, fmt.Errorf("creating the workflow context: %w", err)
	}
	status, _, err := runner.Run(wfCtx, &wftypes.TaskRunOptions{
		PCtx:     pCtx,
		Compiler: compiler,
		// The runner calls providers through the tracer's context, which is
		// where the controller's executor carries their runtime parameters.
		GetTracer: func(string, oamv1alpha1.WorkflowStep) monitorContext.Context {
			return monitorContext.NewTraceContext(goCtx, "")
		},
	})
	if err != nil {
		return nil, err
	}
	return &StepRun{
		Phase: string(status.Phase), Reason: status.Reason, Message: status.Message,
		Calls: rec.calls, Namespace: ns,
	}, nil
}

// readCall is an empty kube.#Read, for expect.resources to fill in.
var readCall = sync.OnceValues(func() (cue.Value, error) {
	compiler, err := testCompiler()
	if err != nil {
		return cue.Value{}, err
	}
	return compiler.CompileStringWithOptions(context.Background(), "import \"vela/kube\"\ncall: kube.#Read", cuex.DisableResolveProviderFunctions{})
})

// object reads back an object a case expects through kube.#Read, as a check
// would, named by apiVersion, kind and metadata.name, in the case's
// namespace unless it names its own; one that does not exist is empty.
func object(want cue.Value, env *callEnv) (map[string]any, error) {
	for _, f := range []string{"apiVersion", "kind", "metadata.name"} {
		if _, err := want.LookupPath(cue.ParsePath(f)).String(); err != nil {
			return nil, fmt.Errorf("%s: an object to read back needs a concrete %s", want.Path(), f)
		}
	}
	id := map[string]any{"metadata": map[string]any{"namespace": env.namespace}}
	for _, f := range []string{"apiVersion", "kind", "metadata.name", "metadata.namespace"} {
		v, err := want.LookupPath(cue.ParsePath(f)).String()
		if err != nil {
			continue
		}
		if name, ok := strings.CutPrefix(f, "metadata."); ok {
			id["metadata"].(map[string]any)[name] = v
		} else {
			id[f] = v
		}
	}
	base, err := readCall()
	if err != nil {
		return nil, err
	}
	resolved, err := resolveCalls(base.FillPath(cue.ParsePath("call.$params.value"), id), cue.ParsePath("call"), env)
	if err != nil {
		return nil, err
	}
	returns := resolved.LookupPath(cue.ParsePath("call.$returns"))
	if msg, _ := returns.LookupPath(cue.ParsePath("err")).String(); msg != "" {
		// The API server's NotFound message, as the provider passes it on.
		if strings.HasSuffix(msg, "not found") {
			return map[string]any{}, nil
		}
		return nil, fmt.Errorf("%s: reading it back: %s", want.Path(), msg)
	}
	obj := map[string]any{}
	if err := returns.LookupPath(cue.ParsePath("value")).Decode(&obj); err != nil {
		return nil, err
	}
	return obj, nil
}

// checksResult runs the case's checks, for real and in order, and returns
// each one's $returns by name.
func (c *Case) checksResult(env *callEnv) (map[string]any, error) {
	path := cue.MakePath(append(c.Expect.Path().Selectors(), cue.Str("checks"))...)
	if !c.root.LookupPath(path).Exists() {
		return nil, nil
	}
	resolved, err := resolveCalls(c.root, path, env)
	if err != nil {
		var ce *callError
		if errors.As(err, &ce) && len(ce.at.Selectors()) > len(path.Selectors()) {
			return nil, caseFailure(fmt.Sprintf("checks.%s: %v", ce.at.Selectors()[len(path.Selectors())], ce.err))
		}
		return nil, err
	}
	out := map[string]any{}
	for it, _ := resolved.LookupPath(path).Fields(); it.Next(); {
		var returns any = map[string]any{}
		if r := it.Value().LookupPath(cue.ParsePath("call.$returns")); r.Exists() {
			if err := r.Decode(&returns); err != nil {
				return nil, caseFailure(fmt.Sprintf("checks.%s: reading $returns: %v", it.Selector(), err))
			}
		}
		out[it.Selector().Unquoted()] = map[string]any{"returns": returns}
	}
	return out, nil
}

// runExec runs the case's step or source between its file's @beforeEach and
// @afterEach hooks, all in the case's namespace, then deletes what the hooks
// and the case's resources created. A case that cannot be set up, or
// whose @beforeEach fails, fails for that without running the definition, so
// no expected error can be met by it; @afterEach runs regardless.
func (c *Case) runExec() []string {
	cl, ns, err := stepCluster(c.File, c.CRDs, c.Input.Context.Namespace)
	if err != nil {
		return []string{setupFailure(err)}
	}
	var before, after []*Hook
	if c.suite != nil {
		before, after = c.suite.BeforeEach, c.suite.AfterEach
	}
	var created createdObjects
	var failures []string
	if err := runBefore(before, cl, ns, &created); err != nil {
		failures = []string{err.Error()}
	} else if err := cl.seed(ns, c.Resources, &created); err != nil {
		failures = []string{setupFailure(err)}
	} else {
		failures = c.check(c.execBody(cl, ns, &created))
	}
	if err := runAfter(after, cl, ns, &created); err != nil {
		failures = append(failures, err.Error())
	}
	return failures
}

// setupFailure is a case failing before its definition ran.
func setupFailure(err error) string {
	return "setting up the case: " + err.Error()
}

// execBody runs what the case tests, a step or a source, against the cluster.
func (c *Case) execBody(cl *TestCluster, ns string, created *createdObjects) (map[string]any, error) {
	if c.Test == TestSourceExec {
		return c.sourceResult(cl, ns)
	}
	return c.execResult(cl, ns, created)
}

func (c *Case) execResult(cl *TestCluster, ns string, created *createdObjects) (map[string]any, error) {
	run, err := runStep(cl, ns, c.Subject, c.Input, nil, created)
	if err != nil {
		return map[string]any{}, err
	}
	result := map[string]any{
		"phase":   run.Phase,
		"reason":  run.Reason,
		"message": run.Message,
		"calls":   callsResult(run.Calls),
	}
	env := &callEnv{cluster: cl, namespace: ns, dir: filepath.Dir(c.File), created: created}
	// From here the step has run: what fails is the harness or the case's
	// expectations, never the step, so no expected error can be met by it.
	checks, err := c.checksResult(env)
	if err != nil {
		var failure caseFailure
		if errors.As(err, &failure) {
			return nil, err
		}
		return nil, caseFailure("running expect.checks: " + err.Error())
	}
	if checks != nil {
		result["checks"] = checks
	}
	if want := c.Expect.LookupPath(cue.ParsePath("resources")); want.Exists() {
		var objs []any
		for _, w := range listElems(want) {
			obj, err := object(w, env)
			if err != nil {
				return nil, caseFailure("reading back expect.resources: " + err.Error())
			}
			objs = append(objs, obj)
		}
		result["resources"] = objs
	}
	return result, nil
}

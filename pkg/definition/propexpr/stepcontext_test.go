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

package propexpr

import (
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	wfprocess "github.com/kubevela/workflow/pkg/cue/process"

	"github.com/oam-dev/kubevela/apis/types"
	velaprocess "github.com/oam-dev/kubevela/pkg/cue/process"
	"github.com/oam-dev/kubevela/pkg/oam"
)

// workflowStepTemplateContext mirrors what a step template runs against:
// appfile.WorkflowContextData, then the step the workflow engine's task runner
// adds (fillContext in the engine's custom task loader).
func workflowStepTemplateContext(t *testing.T) string {
	t.Helper()
	saved := types.ControlPlaneClusterVersion
	types.ControlPlaneClusterVersion = types.ClusterVersion{Major: "1", Minor: "31", GitVersion: "v1.31.0", Platform: "linux/arm64"}
	defer func() { types.ControlPlaneClusterVersion = saved }()

	pCtx := velaprocess.NewContext(velaprocess.ContextData{
		Namespace:       "team-a",
		AppName:         "checkout",
		CompName:        "checkout",
		AppRevisionName: "checkout-v3",
		WorkflowName:    "deploy",
		PublishVersion:  "v1",
		AppLabels:       map[string]string{"team": "payments"},
		AppAnnotations:  map[string]string{oam.AnnotationWorkflowName: "deploy", oam.AnnotationPublishVersion: "v1"},
		Ctx:             publishedContext(),
	})
	wfprocess.NewStepRunTimeMeta().Fill(pCtx, []wfprocess.StepMetaKV{
		wfprocess.WithName("notify"),
		wfprocess.WithSessionID("step-id-1"),
		wfprocess.WithSpanID("span-1"),
	})
	base, err := pCtx.BaseContextFile()
	if err != nil {
		t.Fatalf("building the step template context: %v", err)
	}
	return base
}

// The same drift guard as the other surfaces, against a workflow step's
// template rather than its properties.
func TestWorkflowStepTemplateContextMatchesTheRender(t *testing.T) {
	v := cuecontext.New().CompileString(workflowStepTemplateContext(t))
	if v.Err() != nil {
		t.Fatalf("compiling: %v", v.Err())
	}
	iter, err := v.LookupPath(cue.ParsePath("context")).Fields(cue.All())
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for iter.Next() {
		field := iter.Selector().Unquoted()
		seen[field] = true
		_, readable := WorkflowStepTemplateContext.field(field)
		switch {
		case readable && registry.excluded[field] != "":
			t.Errorf("%q is both readable and globally excluded", field)
		case !readable && !knownField(field):
			t.Errorf("the step template context carries %q (%s) but the registry has never heard of it - "+
				"add it to a group in context.cue, or to excluded with a reason",
				field, iter.Value().IncompleteKind())
		}
		if readable {
			if s, err := iter.Value().String(); err == nil && s == "" {
				t.Errorf("%q is declared readable but is empty in a step template", field)
			}
		}
	}
	for _, field := range WorkflowStepTemplateContext.readable() {
		if !seen[field] {
			t.Errorf("WorkflowStepTemplateContext declares %q, which a step template does not carry", field)
		}
	}
}

// A step template's view is not a place a property expression is
// substituted, so it is not offered as one.
func TestWorkflowStepTemplateIsNotAnExpressionSurface(t *testing.T) {
	if SurfaceDeclared("workflowstep-template") {
		t.Fatal("workflowstep-template is declared as an expression surface")
	}
	for _, s := range SurfacesOffering("stepSessionID") {
		if s == "workflow step templates" {
			t.Fatalf("SurfacesOffering points at workflow step templates: %v", SurfacesOffering("stepSessionID"))
		}
	}
	if _, ok := WorkflowStepTemplateContext.field("stepName"); !ok {
		t.Fatal("the step template's context lost stepName")
	}
}

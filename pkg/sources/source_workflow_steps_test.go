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

package sources

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"

	wfContext "github.com/kubevela/workflow/pkg/context"
	wfprocess "github.com/kubevela/workflow/pkg/cue/process"
	"github.com/kubevela/workflow/pkg/expressions"
	wftypes "github.com/kubevela/workflow/pkg/types"

	velaprocess "github.com/oam-dev/kubevela/pkg/cue/process"
	"github.com/oam-dev/kubevela/pkg/definition/propexpr"
)

// runContext is a workflow context with only its mutable values.
type runContext struct {
	wfContext.Context
	data map[string]string
}

func (c *runContext) GetMutableValue(paths ...string) string { return c.data[strings.Join(paths, ".")] }
func (c *runContext) SetMutableValue(v string, paths ...string) {
	c.data[strings.Join(paths, ".")] = v
}

// stepSources is an Application with one source, cfg, whose value is whatever
// region and replicas are when a step reads it.
type stepSources struct {
	region   string
	replicas int
	revision string
	recorded map[string]map[string]SourceResolutionStatus
}

func (s *stepSources) options() wftypes.ExpressionOptions {
	return StepExpressions(func(step, stepType string) wfprocess.Context {
		return velaprocess.NewContext(velaprocess.ContextData{
			AppName: "checkout", Namespace: "team-a", StepName: step, AppRevisionName: s.revision,
			Sources:     map[string]map[string]interface{}{"cfg": {}},
			SourceTypes: map[string]string{"cfg": "demo"},
			SourceTemplates: map[string]string{"demo": fmt.Sprintf(`
schema: {region: string, replicas: int}
$internal: {key: "demo", keyInputs: []}
output: {region: %q, replicas: %d}
`, s.region, s.replicas)},
		})
	}, func(step, stepType string, statuses map[string]SourceResolutionStatus) {
		s.recorded[step+"/"+stepType] = statuses
	})
}

func (s *stepSources) eval(t *testing.T, run string, wfCtx *runContext, step, props string) (string, error) {
	t.Helper()
	ev, err := expressions.NewEvaluator(s.options(), nil, run)
	require.NoError(t, err)
	out, err := ev.Properties(context.Background(), wftypes.ExpressionStep{Name: step, Type: "notification"},
		&runtime.RawExtension{Raw: []byte(props)}, wfCtx, wfprocess.NewContext(wfprocess.ContextData{}), nil)
	if err != nil {
		return "", err
	}
	return string(out.Raw), nil
}

func TestAStepReadsContextAndSources(t *testing.T) {
	s := &stepSources{region: "eu-west", replicas: 3, recorded: map[string]map[string]SourceResolutionStatus{}}
	out, err := s.eval(t, "run-1", &runContext{data: map[string]string{}}, "notify",
		`{"m":"$(context.appName)/$(context.stepName) in $(source.cfg.region)","n":"$(source.cfg.replicas * 2)"}`)
	require.NoError(t, err)
	require.JSONEq(t, `{"m":"checkout/notify in eu-west","n":6}`, out)
	require.Equal(t, "Resolved", s.recorded["notify/notification"]["cfg"].Phase, "what the step read is recorded against it")
}

func TestASourceHoldsForTheRun(t *testing.T) {
	s := &stepSources{region: "eu-west", replicas: 3, recorded: map[string]map[string]SourceResolutionStatus{}}
	wfCtx := &runContext{data: map[string]string{}}
	props := `{"m":"$(source.cfg.region)","n":"$(source.cfg.replicas * 2)"}`
	_, err := s.eval(t, "run-1", wfCtx, "first", props)
	require.NoError(t, err)

	s.region, s.replicas = "us-east", 5
	out, err := s.eval(t, "run-1", wfCtx, "second", props)
	require.NoError(t, err)
	require.JSONEq(t, `{"m":"eu-west","n":6}`, out, "a later step sees what the first saw, still typed by its schema")
	require.Contains(t, s.recorded, "second/notification", "a step reading a pinned value still records it")
	first, second := s.recorded["first/notification"]["cfg"], s.recorded["second/notification"]["cfg"]
	require.NotEmpty(t, first.Config)
	require.Equal(t, first.Config, second.Config, "and the storage key the value came from")
	require.Equal(t, first.ExpiresAt, second.ExpiresAt)

	out, err = s.eval(t, "run-2", wfCtx, "first", props)
	require.NoError(t, err)
	require.JSONEq(t, `{"m":"us-east","n":10}`, out, "the next run, in the same context, resolves afresh")
}

// A revision is a new run even when it starts in the same second as the last,
// which the run's own token cannot tell apart.
func TestANewRevisionReadsItsSourcesAfresh(t *testing.T) {
	s := &stepSources{region: "eu-west", replicas: 3, revision: "checkout-v1", recorded: map[string]map[string]SourceResolutionStatus{}}
	wfCtx := &runContext{data: map[string]string{}}
	props := `{"m":"$(source.cfg.region)"}`
	_, err := s.eval(t, "2026-10-14T12:00:00Z", wfCtx, "first", props)
	require.NoError(t, err)

	s.region, s.revision = "us-east", "checkout-v2"
	out, err := s.eval(t, "2026-10-14T12:00:00Z", wfCtx, "first", props)
	require.NoError(t, err)
	require.JSONEq(t, `{"m":"us-east"}`, out)
}

// A value pinned when it was served stale says so to every later step that
// reads it, as it did to the first.
func TestAPinKeepsWhyItsValueWasServed(t *testing.T) {
	s := &stepSources{region: "us-east", replicas: 3, recorded: map[string]map[string]SourceResolutionStatus{}}
	wfCtx := &runContext{data: map[string]string{pinnedSourcesKey: `{"run":"run-1","values":{"cfg":{"region":"eu-west","replicas":3}},` +
		`"from":{"cfg":{"config":"demo-abc","message":"serving the stored value: refresh failed: timeout"}}}`}}
	out, err := s.eval(t, "run-1", wfCtx, "second", `{"m":"$(source.cfg.region)"}`)
	require.NoError(t, err)
	require.JSONEq(t, `{"m":"eu-west"}`, out)
	got := s.recorded["second/notification"]["cfg"]
	require.Equal(t, "demo-abc", got.Config)
	require.Contains(t, got.Message, "serving the stored value: refresh failed: timeout")
	require.Contains(t, got.Message, "pinned for this workflow run")
}

func TestStepExpressionsNeedAWorkflow(t *testing.T) {
	s := &stepSources{region: "eu-west", recorded: map[string]map[string]SourceResolutionStatus{}}
	_, err := s.options().Resolvers[propexpr.SourceIdent].Resolve(context.Background(), nil)
	require.ErrorContains(t, err, "workflow step")
}

// A read that fails does not undo the ones that succeeded: what they resolved is
// pinned, so the step's retry, and every later step, reads the same value.
func TestAFailedReadKeepsTheSourcesThatResolved(t *testing.T) {
	s := &stepSources{region: "eu-west", replicas: 3, recorded: map[string]map[string]SourceResolutionStatus{}}
	wfCtx := &runContext{data: map[string]string{}}
	_, err := s.eval(t, "run-1", wfCtx, "first", `{"m":"$(source.cfg.region)","x":"$(source.missing.region)"}`)
	require.Error(t, err)

	s.region = "us-east"
	out, err := s.eval(t, "run-1", wfCtx, "second", `{"m":"$(source.cfg.region)"}`)
	require.NoError(t, err)
	require.JSONEq(t, `{"m":"eu-west"}`, out)
}

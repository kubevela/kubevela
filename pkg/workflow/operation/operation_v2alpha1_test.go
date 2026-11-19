/*
 Copyright 2026. The KubeVela Authors.

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

package operation

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	oamv1alpha1 "github.com/kubevela/pkg/apis/oam/v1alpha1"
	workflowv1alpha1 "github.com/kubevela/workflow/api/v1alpha1"
	wfTypes "github.com/kubevela/workflow/pkg/types"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v2alpha1"
)

// wrapSteps is the test-side counterpart to the package's own unwrapSteps --
// builds fixture Steps from plain engine-shaped WorkflowStepStatus entries.
func wrapSteps(steps ...workflowv1alpha1.WorkflowStepStatus) []v2alpha1.OperationWorkflowStepStatus {
	out := make([]v2alpha1.OperationWorkflowStepStatus, len(steps))
	for i, s := range steps {
		out[i] = v2alpha1.OperationWorkflowStepStatus{WorkflowStepStatus: s}
	}
	return out
}

// engineShape extracts the subset of ws that used to live on the inlined
// WorkflowRunStatus, for tests that assert against that shape directly.
func engineShape(ws v2alpha1.OperationWorkflowStatus) workflowv1alpha1.WorkflowRunStatus {
	return workflowv1alpha1.WorkflowRunStatus{Suspend: ws.Suspend, Steps: unwrapSteps(ws.Steps)}
}

func operationTestClient(t *testing.T) client.Client {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, v2alpha1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))
	return fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&v2alpha1.Operation{}).
		Build()
}

func TestOperationSuspend(t *testing.T) {
	ctx := context.Background()

	testCases := map[string]struct {
		op          *v2alpha1.Operation
		step        string
		expected    workflowv1alpha1.WorkflowRunStatus
		expectedErr string
	}{
		"terminated": {
			op: &v2alpha1.Operation{
				ObjectMeta: metav1.ObjectMeta{Name: "terminated"},
				Status:     v2alpha1.OperationStatus{Phase: v2alpha1.OperationPhaseSucceeded},
			},
			expectedErr: "can not suspend a terminated operation",
		},
		"suspend all": {
			op: &v2alpha1.Operation{
				ObjectMeta: metav1.ObjectMeta{Name: "suspend-all"},
				Status: v2alpha1.OperationStatus{
					Phase: v2alpha1.OperationPhaseRunning,
					Workflows: []v2alpha1.OperationWorkflowStatus{{
						Steps: wrapSteps(
							workflowv1alpha1.WorkflowStepStatus{
								StepStatus: workflowv1alpha1.StepStatus{Name: "step1", Phase: workflowv1alpha1.WorkflowStepPhaseRunning},
								SubStepsStatus: []workflowv1alpha1.StepStatus{
									{Name: "sub1", Phase: workflowv1alpha1.WorkflowStepPhaseRunning},
								},
							},
							workflowv1alpha1.WorkflowStepStatus{
								StepStatus: workflowv1alpha1.StepStatus{Name: "step2", Phase: workflowv1alpha1.WorkflowStepPhaseSucceeded},
							},
						),
					}},
				},
			},
			expected: workflowv1alpha1.WorkflowRunStatus{
				Suspend: true,
				Steps: []workflowv1alpha1.WorkflowStepStatus{
					{
						StepStatus: workflowv1alpha1.StepStatus{Name: "step1", Phase: workflowv1alpha1.WorkflowStepPhaseSuspending},
						SubStepsStatus: []workflowv1alpha1.StepStatus{
							{Name: "sub1", Phase: workflowv1alpha1.WorkflowStepPhaseSuspending},
						},
					},
					{
						StepStatus: workflowv1alpha1.StepStatus{Name: "step2", Phase: workflowv1alpha1.WorkflowStepPhaseSucceeded},
					},
				},
			},
		},
		"suspend specific step": {
			step: "step1",
			op: &v2alpha1.Operation{
				ObjectMeta: metav1.ObjectMeta{Name: "suspend-step"},
				Status: v2alpha1.OperationStatus{
					Phase: v2alpha1.OperationPhaseRunning,
					Workflows: []v2alpha1.OperationWorkflowStatus{{
						Steps: wrapSteps(
							workflowv1alpha1.WorkflowStepStatus{StepStatus: workflowv1alpha1.StepStatus{Name: "step1", Phase: workflowv1alpha1.WorkflowStepPhaseRunning}},
							workflowv1alpha1.WorkflowStepStatus{StepStatus: workflowv1alpha1.StepStatus{Name: "step2", Phase: workflowv1alpha1.WorkflowStepPhaseRunning}},
						),
					}},
				},
			},
			expected: workflowv1alpha1.WorkflowRunStatus{
				Suspend: true,
				Steps: []workflowv1alpha1.WorkflowStepStatus{
					{StepStatus: workflowv1alpha1.StepStatus{Name: "step1", Phase: workflowv1alpha1.WorkflowStepPhaseSuspending}},
					{StepStatus: workflowv1alpha1.StepStatus{Name: "step2", Phase: workflowv1alpha1.WorkflowStepPhaseRunning}},
				},
			},
		},
		"step not found": {
			step: "missing",
			op: &v2alpha1.Operation{
				ObjectMeta: metav1.ObjectMeta{Name: "step-not-found"},
				Status: v2alpha1.OperationStatus{
					Phase:     v2alpha1.OperationPhaseRunning,
					Workflows: []v2alpha1.OperationWorkflowStatus{{}},
				},
			},
			expectedErr: "can not find step missing",
		},
		"no workflow started yet": {
			op: &v2alpha1.Operation{
				ObjectMeta: metav1.ObjectMeta{Name: "not-started"},
				Status:     v2alpha1.OperationStatus{Phase: v2alpha1.OperationPhasePending},
			},
			expectedErr: "has not started a workflow yet",
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			r := require.New(t)
			cli := operationTestClient(t)
			r.NoError(cli.Create(ctx, tc.op))

			var err error
			if tc.step == "" {
				err = NewOperationWorkflowOperator(cli, nil, tc.op).Suspend(ctx)
			} else {
				err = NewOperationWorkflowStepOperator(cli, nil, tc.op).Suspend(ctx, tc.step)
			}
			if tc.expectedErr != "" {
				r.Error(err)
				r.Contains(err.Error(), tc.expectedErr)
				return
			}
			r.NoError(err)

			got := &v2alpha1.Operation{}
			r.NoError(cli.Get(ctx, client.ObjectKeyFromObject(tc.op), got))
			r.Equal(tc.expected, engineShape(got.Status.Workflows[0]))
		})
	}
}

func TestOperationResume(t *testing.T) {
	ctx := context.Background()

	testCases := map[string]struct {
		op          *v2alpha1.Operation
		step        string
		expected    workflowv1alpha1.WorkflowRunStatus
		expectedErr string
	}{
		"not suspended": {
			// A hard error, not a silent no-op: the CLI unconditionally
			// polls to terminal after Resume returns nil, so no-op'ing here
			// would let that poll misreport some unrelated later
			// completion as if this resume caused it.
			op: &v2alpha1.Operation{
				ObjectMeta: metav1.ObjectMeta{Name: "not-suspended"},
				Status: v2alpha1.OperationStatus{
					Phase:     v2alpha1.OperationPhaseRunning,
					Workflows: []v2alpha1.OperationWorkflowStatus{{}},
				},
			},
			expectedErr: "is not suspended",
		},
		"resume all": {
			op: &v2alpha1.Operation{
				ObjectMeta: metav1.ObjectMeta{Name: "resume-all"},
				Status: v2alpha1.OperationStatus{
					Phase: v2alpha1.OperationPhaseSuspended,
					Workflows: []v2alpha1.OperationWorkflowStatus{{
						Suspend: true,
						Steps: wrapSteps(
							workflowv1alpha1.WorkflowStepStatus{StepStatus: workflowv1alpha1.StepStatus{Name: "step1", Phase: workflowv1alpha1.WorkflowStepPhaseSuspending}},
						),
					}},
				},
			},
			expected: workflowv1alpha1.WorkflowRunStatus{
				Steps: []workflowv1alpha1.WorkflowStepStatus{
					{StepStatus: workflowv1alpha1.StepStatus{Name: "step1", Phase: workflowv1alpha1.WorkflowStepPhaseRunning}},
				},
			},
		},
		"resume specific step": {
			step: "step1",
			op: &v2alpha1.Operation{
				ObjectMeta: metav1.ObjectMeta{Name: "resume-step"},
				Status: v2alpha1.OperationStatus{
					Phase: v2alpha1.OperationPhaseSuspended,
					Workflows: []v2alpha1.OperationWorkflowStatus{{
						Suspend: true,
						Steps: wrapSteps(
							workflowv1alpha1.WorkflowStepStatus{StepStatus: workflowv1alpha1.StepStatus{Name: "step1", Phase: workflowv1alpha1.WorkflowStepPhaseSuspending}},
							workflowv1alpha1.WorkflowStepStatus{StepStatus: workflowv1alpha1.StepStatus{Name: "step2", Phase: workflowv1alpha1.WorkflowStepPhaseSuspending}},
						),
					}},
				},
			},
			expected: workflowv1alpha1.WorkflowRunStatus{
				Steps: []workflowv1alpha1.WorkflowStepStatus{
					{StepStatus: workflowv1alpha1.StepStatus{Name: "step1", Phase: workflowv1alpha1.WorkflowStepPhaseRunning}},
					{StepStatus: workflowv1alpha1.StepStatus{Name: "step2", Phase: workflowv1alpha1.WorkflowStepPhaseSuspending}},
				},
			},
		},
		"step not found": {
			step: "missing",
			op: &v2alpha1.Operation{
				ObjectMeta: metav1.ObjectMeta{Name: "resume-not-found"},
				Status: v2alpha1.OperationStatus{
					Phase:     v2alpha1.OperationPhaseSuspended,
					Workflows: []v2alpha1.OperationWorkflowStatus{{Suspend: true}},
				},
			},
			expectedErr: "can not find step missing",
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			r := require.New(t)
			cli := operationTestClient(t)
			r.NoError(cli.Create(ctx, tc.op))

			var err error
			if tc.step == "" {
				err = NewOperationWorkflowOperator(cli, nil, tc.op).Resume(ctx)
			} else {
				err = NewOperationWorkflowStepOperator(cli, nil, tc.op).Resume(ctx, tc.step)
			}
			if tc.expectedErr != "" {
				r.Error(err)
				r.Contains(err.Error(), tc.expectedErr)
				return
			}
			r.NoError(err)

			got := &v2alpha1.Operation{}
			r.NoError(cli.Get(ctx, client.ObjectKeyFromObject(tc.op), got))
			r.False(got.Status.Workflows[0].Suspend)
			r.Equal(tc.expected, engineShape(got.Status.Workflows[0]))
		})
	}
}

func TestOperationRollback(t *testing.T) {
	err := NewOperationWorkflowOperator(nil, nil, nil).Rollback(context.Background())
	require.EqualError(t, err, "cannot rollback an Operation")
}

func TestOperationTerminate(t *testing.T) {
	ctx := context.Background()
	r := require.New(t)
	cli := operationTestClient(t)
	op := &v2alpha1.Operation{
		ObjectMeta: metav1.ObjectMeta{Name: "terminate-me"},
		Status: v2alpha1.OperationStatus{
			Phase: v2alpha1.OperationPhaseRunning,
			Workflows: []v2alpha1.OperationWorkflowStatus{{
				Suspend: true,
				Steps: []v2alpha1.OperationWorkflowStepStatus{
					{WorkflowStepStatus: workflowv1alpha1.WorkflowStepStatus{
						StepStatus:     workflowv1alpha1.StepStatus{Name: "running-step", Phase: workflowv1alpha1.WorkflowStepPhaseRunning},
						SubStepsStatus: []workflowv1alpha1.StepStatus{{Name: "suspending-sub", Phase: workflowv1alpha1.WorkflowStepPhaseSuspending}},
					}},
					{WorkflowStepStatus: workflowv1alpha1.WorkflowStepStatus{StepStatus: workflowv1alpha1.StepStatus{
						Name: "already-succeeded", Phase: workflowv1alpha1.WorkflowStepPhaseSucceeded,
					}}},
				},
			}},
		},
	}
	r.NoError(cli.Create(ctx, op))

	r.NoError(NewOperationWorkflowOperator(cli, nil, op).Terminate(ctx))

	got := &v2alpha1.Operation{}
	r.NoError(cli.Get(ctx, client.ObjectKeyFromObject(op), got))
	r.Equal(v2alpha1.OperationPhaseFailed, got.Status.Phase)
	r.NotNil(got.Status.CompletionTime, "a terminated Operation must be eligible for TTL cleanup immediately")
	r.True(got.Status.Workflows[0].Terminated)
	r.False(got.Status.Workflows[0].Suspend)

	steps := got.Status.Workflows[0].Steps
	r.Equal(workflowv1alpha1.WorkflowStepPhaseFailed, steps[0].Phase, "a still-Running step must be flipped to Failed, not left reading as in-progress on a terminal Operation")
	r.Equal(wfTypes.StatusReasonTerminate, steps[0].Reason)
	r.Equal(workflowv1alpha1.WorkflowStepPhaseFailed, steps[0].SubStepsStatus[0].Phase, "a still-Suspending sub-step must likewise be flipped to Failed")
	r.Equal(wfTypes.StatusReasonTerminate, steps[0].SubStepsStatus[0].Reason)
	r.Equal(workflowv1alpha1.WorkflowStepPhaseSucceeded, steps[1].Phase, "an already-terminal step must be left alone")
}

func TestOperationRestartGuards(t *testing.T) {
	ctx := context.Background()

	t.Run("cannot restart a running operation", func(t *testing.T) {
		r := require.New(t)
		cli := operationTestClient(t)
		op := &v2alpha1.Operation{
			ObjectMeta: metav1.ObjectMeta{Name: "running"},
			Status:     v2alpha1.OperationStatus{Phase: v2alpha1.OperationPhaseRunning},
		}
		r.NoError(cli.Create(ctx, op))
		err := NewOperationWorkflowOperator(cli, nil, op).Restart(ctx)
		r.ErrorContains(err, "cannot restart a running operation")
	})

	t.Run("no workflow started yet", func(t *testing.T) {
		r := require.New(t)
		cli := operationTestClient(t)
		op := &v2alpha1.Operation{
			ObjectMeta: metav1.ObjectMeta{Name: "pending"},
			Status:     v2alpha1.OperationStatus{Phase: v2alpha1.OperationPhasePending},
		}
		r.NoError(cli.Create(ctx, op))
		err := NewOperationWorkflowOperator(cli, nil, op).Restart(ctx)
		r.ErrorContains(err, "has not started a workflow yet")
	})
}

func TestOperationRestartWholeWorkflow(t *testing.T) {
	ctx := context.Background()
	r := require.New(t)
	cli := operationTestClient(t)

	op := &v2alpha1.Operation{
		ObjectMeta: metav1.ObjectMeta{Name: "whole-restart", Namespace: "default"},
		Status: v2alpha1.OperationStatus{
			Phase:          v2alpha1.OperationPhaseFailed,
			Message:        "boom",
			Attempts:       1,
			Template:       &v2alpha1.OperationTemplateSpec{},
			CompletionTime: &metav1.Time{},
			Workflows: []v2alpha1.OperationWorkflowStatus{{
				Cluster:        "local",
				Terminated:     true,
				Finished:       true,
				ContextBackend: &corev1.ObjectReference{Name: "whole-restart-context", Namespace: "default"},
				// .Attempts pre-populated on each entry, simulating what the
				// controller's recordStepCompletion already logged when
				// each step first finished (this test only exercises
				// restartFrom's own snapshot-then-wipe logic, not that
				// controller-side recording).
				Steps: []v2alpha1.OperationWorkflowStepStatus{
					{
						WorkflowStepStatus: workflowv1alpha1.WorkflowStepStatus{StepStatus: workflowv1alpha1.StepStatus{Name: "step1", Phase: workflowv1alpha1.WorkflowStepPhaseSucceeded, Message: "ok"}},
						Attempts:           []v2alpha1.OperationStepAttempt{{AttemptNumber: 1, Phase: workflowv1alpha1.WorkflowStepPhaseSucceeded, Message: "ok"}},
					},
					{
						WorkflowStepStatus: workflowv1alpha1.WorkflowStepStatus{StepStatus: workflowv1alpha1.StepStatus{Name: "step2", Phase: workflowv1alpha1.WorkflowStepPhaseFailed, Message: "failed hard"}},
						Attempts:           []v2alpha1.OperationStepAttempt{{AttemptNumber: 1, Phase: workflowv1alpha1.WorkflowStepPhaseFailed, Message: "failed hard"}},
					},
				},
			}},
		},
	}
	r.NoError(cli.Create(ctx, op))
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "whole-restart-context", Namespace: "default"},
		Data:       map[string]string{"vars": "{}"},
	}
	r.NoError(cli.Create(ctx, cm))

	r.NoError(NewOperationWorkflowOperator(cli, nil, op).Restart(ctx))

	got := &v2alpha1.Operation{}
	r.NoError(cli.Get(ctx, client.ObjectKeyFromObject(op), got))
	r.Equal(v2alpha1.OperationPhaseRunning, got.Status.Phase)
	r.Nil(got.Status.CompletionTime)
	r.Empty(got.Status.Message, "a restart must clear the prior run's failure message, not leave it stuck")
	r.EqualValues(2, got.Status.Attempts)
	gotWS := got.Status.Workflows[0]
	r.False(gotWS.Terminated)
	r.False(gotWS.Finished)
	r.False(gotWS.Suspend)
	r.Nil(gotWS.ContextBackend)
	r.Empty(gotWS.Steps)
	r.True(gotWS.StartTime.IsZero())
	r.True(gotWS.EndTime.IsZero())

	// Prior attempts are preserved even though the live Steps were wiped.
	r.Equal([]v2alpha1.OperationStepAttempt{{AttemptNumber: 1, Phase: workflowv1alpha1.WorkflowStepPhaseSucceeded, Message: "ok"}},
		got.Status.Workflows[0].StepAttempts["step1"])
	r.Equal([]v2alpha1.OperationStepAttempt{{AttemptNumber: 1, Phase: workflowv1alpha1.WorkflowStepPhaseFailed, Message: "failed hard"}},
		got.Status.Workflows[0].StepAttempts["step2"])

	// The old context-backend ConfigMap is deleted, not just orphaned.
	err := cli.Get(ctx, client.ObjectKeyFromObject(cm), &corev1.ConfigMap{})
	r.True(kerrors.IsNotFound(err))
}

func TestOperationRestartFromStep(t *testing.T) {
	ctx := context.Background()

	steps := []oamv1alpha1.WorkflowStep{
		{WorkflowStepBase: oamv1alpha1.WorkflowStepBase{Name: "step1", Outputs: oamv1alpha1.StepOutputs{{Name: "step1-output1"}}}},
		{WorkflowStepBase: oamv1alpha1.WorkflowStepBase{Name: "step2", Outputs: oamv1alpha1.StepOutputs{{Name: "step2-output1"}}}},
		{WorkflowStepBase: oamv1alpha1.WorkflowStepBase{Name: "step3", Outputs: oamv1alpha1.StepOutputs{{Name: "step3-output1"}}}},
	}
	vars := map[string]any{
		"step1-output1": "step1-output1",
		"step2-output1": "step2-output1",
		"step3-output1": "step3-output1",
	}

	newOp := func(name string, phase workflowv1alpha1.WorkflowStepPhase) *v2alpha1.Operation {
		return &v2alpha1.Operation{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
			Status: v2alpha1.OperationStatus{
				Phase:    v2alpha1.OperationPhaseFailed,
				Template: &v2alpha1.OperationTemplateSpec{Workflow: oamv1alpha1.WorkflowSpec{Steps: steps}},
				Workflows: []v2alpha1.OperationWorkflowStatus{{
					ContextBackend: &corev1.ObjectReference{Name: name + "-context", Namespace: "default"},
					// A real terminal run leaves these set -- a --step
					// restart has to clear them itself (see the
					// "resets whole-run flags" subtest below), the same
					// way the whole-workflow path's full wipe does
					// implicitly.
					Terminated:   true,
					Finished:     true,
					Suspend:      true,
					Phase:        workflowv1alpha1.WorkflowStateFailed,
					Message:      "boom",
					SuspendState: "some-suspend-state",
					EndTime:      metav1.Time{Time: time.Now()},
					// .Attempts pre-populated on each entry, simulating
					// what the controller's recordStepCompletion already
					// logged when each step first finished.
					Steps: []v2alpha1.OperationWorkflowStepStatus{
						{
							WorkflowStepStatus: workflowv1alpha1.WorkflowStepStatus{StepStatus: workflowv1alpha1.StepStatus{Name: "step1", Phase: workflowv1alpha1.WorkflowStepPhaseSucceeded}},
							Attempts:           []v2alpha1.OperationStepAttempt{{AttemptNumber: 1, Phase: workflowv1alpha1.WorkflowStepPhaseSucceeded}},
						},
						{
							WorkflowStepStatus: workflowv1alpha1.WorkflowStepStatus{StepStatus: workflowv1alpha1.StepStatus{Name: "step2", Phase: phase}},
							Attempts:           []v2alpha1.OperationStepAttempt{{AttemptNumber: 1, Phase: phase}},
						},
						{
							WorkflowStepStatus: workflowv1alpha1.WorkflowStepStatus{StepStatus: workflowv1alpha1.StepStatus{Name: "step3", Phase: workflowv1alpha1.WorkflowStepPhaseFailed}},
							Attempts:           []v2alpha1.OperationStepAttempt{{AttemptNumber: 1, Phase: workflowv1alpha1.WorkflowStepPhaseFailed}},
						},
					},
				}},
			},
		}
	}

	// Deliberate divergence from upstream CleanStatusFromStep: restarting a
	// Succeeded (or Skipped) step is allowed, not just a Failed one.
	for _, phase := range []workflowv1alpha1.WorkflowStepPhase{
		workflowv1alpha1.WorkflowStepPhaseFailed,
		workflowv1alpha1.WorkflowStepPhaseSucceeded,
		workflowv1alpha1.WorkflowStepPhaseSkipped,
	} {
		t.Run("restarts step regardless of phase "+string(phase), func(t *testing.T) {
			r := require.New(t)
			cli := operationTestClient(t)
			op := newOp("restart-step-"+string(phase), phase)
			r.NoError(cli.Create(ctx, op))

			b, err := json.Marshal(vars)
			r.NoError(err)
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: op.Name + "-context", Namespace: "default"},
				Data:       map[string]string{"vars": string(b)},
			}
			r.NoError(cli.Create(ctx, cm))

			r.NoError(NewOperationWorkflowStepOperator(cli, nil, op).Restart(ctx, "step2"))

			got := &v2alpha1.Operation{}
			r.NoError(cli.Get(ctx, client.ObjectKeyFromObject(op), got))

			// step1 (before the restart point), including its already-
			// recorded attempt, is untouched; step2 and step3 (the target
			// and everything after it, in sequential mode) are gone from
			// the live status -- they'll re-run from scratch.
			r.Equal([]v2alpha1.OperationWorkflowStepStatus{
				{
					WorkflowStepStatus: workflowv1alpha1.WorkflowStepStatus{StepStatus: workflowv1alpha1.StepStatus{Name: "step1", Phase: workflowv1alpha1.WorkflowStepPhaseSucceeded}},
					Attempts:           []v2alpha1.OperationStepAttempt{{AttemptNumber: 1, Phase: workflowv1alpha1.WorkflowStepPhaseSucceeded}},
				},
			}, got.Status.Workflows[0].Steps)

			r.Len(got.Status.Workflows[0].StepAttempts["step2"], 1)
			r.Equal(phase, got.Status.Workflows[0].StepAttempts["step2"][0].Phase)
			r.Len(got.Status.Workflows[0].StepAttempts["step3"], 1)

			// step1's recorded output survives; step2 and step3's are cleared,
			// so a restarted step2 still sees step1's output via `inputs`.
			gotCM := &corev1.ConfigMap{}
			r.NoError(cli.Get(ctx, client.ObjectKey{Name: op.Name + "-context", Namespace: "default"}, gotCM))
			r.JSONEq(`{"step1-output1":"step1-output1"}`, gotCM.Data["vars"])

			r.EqualValues(1, got.Status.Attempts)
			r.Equal(v2alpha1.OperationPhaseRunning, got.Status.Phase)

			// Regression: a --step restart must clear the whole-run
			// Terminated/Suspend/Finished/EndTime flags left by the prior
			// terminal run, exactly like RestartFromStep does upstream --
			// otherwise the engine reports the run terminated/failed again
			// on the very next reconcile no matter how the restarted step
			// itself turns out.
			ws := got.Status.Workflows[0]
			r.False(ws.Terminated, "Terminated must be cleared by a --step restart")
			r.False(ws.Suspend, "Suspend must be cleared by a --step restart")
			r.False(ws.Finished, "Finished must be cleared by a --step restart")
			r.True(ws.EndTime.IsZero(), "EndTime must be cleared by a --step restart")
			r.Empty(ws.Phase, "Phase must be cleared by a --step restart, not left at the prior run's terminal phase")
			r.Empty(ws.Message, "Message must be cleared by a --step restart, not left at the prior run's failure message")
			r.Empty(ws.SuspendState, "SuspendState must be cleared by a --step restart")
		})
	}

	t.Run("tolerates an already-deleted context-backend ConfigMap", func(t *testing.T) {
		r := require.New(t)
		cli := operationTestClient(t)
		op := newOp("restart-step-no-configmap", workflowv1alpha1.WorkflowStepPhaseFailed)
		r.NoError(cli.Create(ctx, op))
		// Deliberately not creating the context-backend ConfigMap the
		// status references -- it may already be gone (e.g. GC'd), and
		// that must not abort the restart.

		r.NoError(NewOperationWorkflowStepOperator(cli, nil, op).Restart(ctx, "step2"))

		got := &v2alpha1.Operation{}
		r.NoError(cli.Get(ctx, client.ObjectKeyFromObject(op), got))
		r.Equal(v2alpha1.OperationPhaseRunning, got.Status.Phase)
		r.Len(got.Status.Workflows[0].Steps, 1)
	})

	t.Run("step not found", func(t *testing.T) {
		r := require.New(t)
		cli := operationTestClient(t)
		op := newOp("restart-missing-step", workflowv1alpha1.WorkflowStepPhaseFailed)
		r.NoError(cli.Create(ctx, op))
		err := NewOperationWorkflowStepOperator(cli, nil, op).Restart(ctx, "no-such-step")
		r.ErrorContains(err, "no-such-step not found")
	})

	// Regression: a step restarted a *second* time must carry forward its
	// full attempt history (1, 2, ...), not just the latest one --
	// numbering itself is recordStepCompletion's job
	// (pkg/controller/.../operation/generator.go, covered by
	// TestRecordStepCompletion there); snapshotAttempts here only needs to
	// preserve whatever it's handed.
	t.Run("second restart of the same step: full attempt history carried forward, not just the latest", func(t *testing.T) {
		r := require.New(t)
		cli := operationTestClient(t)
		op := newOp("restart-twice", workflowv1alpha1.WorkflowStepPhaseFailed)
		r.NoError(cli.Create(ctx, op))

		b, err := json.Marshal(vars)
		r.NoError(err)
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: op.Name + "-context", Namespace: "default"},
			Data:       map[string]string{"vars": string(b)},
		}
		r.NoError(cli.Create(ctx, cm))

		r.NoError(NewOperationWorkflowStepOperator(cli, nil, op).Restart(ctx, "step2"))

		got := &v2alpha1.Operation{}
		r.NoError(cli.Get(ctx, client.ObjectKeyFromObject(op), got))
		r.Len(got.Status.Workflows[0].StepAttempts["step2"], 1)

		// Simulate the controller's reconcile in between: step2 reappears,
		// fails again, and recordStepCompletion appends attempt 2 onto the
		// pending attempt 1 it reattached -- exactly what
		// operationWorkflowStatusFromEngine does.
		firstAttempt := got.Status.Workflows[0].StepAttempts["step2"]
		got.Status.Workflows[0].Steps = append(got.Status.Workflows[0].Steps, v2alpha1.OperationWorkflowStepStatus{
			WorkflowStepStatus: workflowv1alpha1.WorkflowStepStatus{StepStatus: workflowv1alpha1.StepStatus{Name: "step2", Phase: workflowv1alpha1.WorkflowStepPhaseFailed}},
			Attempts:           append(firstAttempt, v2alpha1.OperationStepAttempt{AttemptNumber: 2, Phase: workflowv1alpha1.WorkflowStepPhaseFailed}),
		})
		got.Status.Workflows[0].StepAttempts = nil
		got.Status.Phase = v2alpha1.OperationPhaseFailed
		r.NoError(cli.Status().Update(ctx, got))

		r.NoError(NewOperationWorkflowStepOperator(cli, nil, got).Restart(ctx, "step2"))

		final := &v2alpha1.Operation{}
		r.NoError(cli.Get(ctx, client.ObjectKeyFromObject(op), final))
		attempts := final.Status.Workflows[0].StepAttempts["step2"]
		require.Len(t, attempts, 2)
		assert.EqualValues(t, 1, attempts[0].AttemptNumber)
		assert.EqualValues(t, 2, attempts[1].AttemptNumber)
	})
}

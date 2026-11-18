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

package cli

import (
	"context"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v2alpha1"
	velaargs "github.com/oam-dev/kubevela/pkg/utils/common"
	"github.com/oam-dev/kubevela/pkg/utils/util"
)

// TestOperationCommandsRegisterEnvFlag guards against GetNamespaceFromEnv
// panicking on a nil "env" flag when --namespace is omitted.
func TestOperationCommandsRegisterEnvFlag(t *testing.T) {
	args := velaargs.Args{}
	io := util.IOStreams{}
	for _, cmd := range []*cobra.Command{
		NewOperationListCommand(args, io),
		NewOperationRunCommand(args, io),
		NewOperationStatusCommand(args, io),
		NewOperationRestartCommand(args, io),
		NewOperationResumeCommand(args, io),
		NewOperationSuspendCommand(args, io),
	} {
		flag := cmd.Flag("env")
		require.NotNil(t, flag, "%s must register --env (via addNamespaceAndEnvArg)", cmd.Name())
		assert.NotPanics(t, func() {
			_ = flag.Value.String()
		})
	}
}

func TestValidateOperationClusterFlag(t *testing.T) {
	assert.NoError(t, validateOperationClusterFlag(""))
	assert.NoError(t, validateOperationClusterFlag("local"))
	err := validateOperationClusterFlag("remote")
	assert.ErrorContains(t, err, `only supports "local" so far, got "remote"`)
}

func TestGetOperationByName(t *testing.T) {
	scheme := newOperationTestScheme(t)
	op := &v2alpha1.Operation{ObjectMeta: metav1.ObjectMeta{Name: "exists", Namespace: "default"}}
	cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(op).Build()

	got, err := getOperationByName(context.Background(), cli, "default", "exists")
	require.NoError(t, err)
	assert.Equal(t, "exists", got.Name)

	_, err = getOperationByName(context.Background(), cli, "default", "missing")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `operation "missing" not found in namespace "default"`)
}

// TestPollOperationUntilTerminalAlreadyDone guards the bounded, no-hang
// happy path: when the Operation is already terminal on the first Get,
// pollOperationUntilTerminal must return immediately rather than sleeping.
func TestPollOperationUntilTerminalAlreadyDone(t *testing.T) {
	scheme := newOperationTestScheme(t)
	cmd := &cobra.Command{}
	initCommand(cmd)

	t.Run("succeeded", func(t *testing.T) {
		op := &v2alpha1.Operation{
			ObjectMeta: metav1.ObjectMeta{Name: "done-ok", Namespace: "default"},
			Status:     v2alpha1.OperationStatus{Phase: v2alpha1.OperationPhaseSucceeded},
		}
		cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(op).Build()
		err := pollOperationUntilTerminal(context.Background(), cmd, cli, op)
		require.NoError(t, err)
	})

	t.Run("failed", func(t *testing.T) {
		op := &v2alpha1.Operation{
			ObjectMeta: metav1.ObjectMeta{Name: "done-failed", Namespace: "default"},
			Status:     v2alpha1.OperationStatus{Phase: v2alpha1.OperationPhaseFailed, Message: "boom"},
		}
		cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(op).Build()
		err := pollOperationUntilTerminal(context.Background(), cmd, cli, op)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "boom")
	})
}

// TestPollOperationUntilSuspendedAlreadyDone mirrors the above for suspend:
// bounded happy paths only -- exercising the "does it actually wait for
// Suspended rather than any change" property needs a controller actually
// driving state over time, which belongs in the e2e suite, not a
// fake-client unit test that would otherwise hang forever waiting for
// a transition nothing will ever make.
func TestPollOperationUntilSuspendedAlreadyDone(t *testing.T) {
	scheme := newOperationTestScheme(t)
	cmd := &cobra.Command{}
	initCommand(cmd)

	t.Run("already suspended", func(t *testing.T) {
		op := &v2alpha1.Operation{
			ObjectMeta: metav1.ObjectMeta{Name: "already-suspended", Namespace: "default"},
			Status:     v2alpha1.OperationStatus{Phase: v2alpha1.OperationPhaseSuspended},
		}
		cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(op).Build()
		require.NoError(t, pollOperationUntilSuspended(context.Background(), cmd, cli, op))
	})

	t.Run("finished before ever suspending", func(t *testing.T) {
		op := &v2alpha1.Operation{
			ObjectMeta: metav1.ObjectMeta{Name: "raced-to-terminal", Namespace: "default"},
			Status:     v2alpha1.OperationStatus{Phase: v2alpha1.OperationPhaseSucceeded},
		}
		cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(op).Build()
		require.NoError(t, pollOperationUntilSuspended(context.Background(), cmd, cli, op))
	})
}

func TestValidateOperationRestartOnlyFlag(t *testing.T) {
	assert.NoError(t, validateOperationRestartOnlyFlag("", false), "no flags at all: fine")
	assert.NoError(t, validateOperationRestartOnlyFlag("step1", false), "--step without --only: fine")

	err := validateOperationRestartOnlyFlag("", true)
	assert.ErrorContains(t, err, "--only requires --step")

	// --only is rejected outright, not silently downgraded to a full
	// cascading restart -- see validateOperationRestartOnlyFlag's own doc.
	err = validateOperationRestartOnlyFlag("step1", true)
	assert.ErrorContains(t, err, "not implemented")
	assert.ErrorContains(t, err, "step1")
}

func TestValidateOperationSourceFlags(t *testing.T) {
	newCmd := func() *cobra.Command {
		cmd := &cobra.Command{Use: "run"}
		cmd.Flags().StringP(FlagComponent, "c", "", "")
		cmd.Flags().String(FlagApplication, "", "")
		return cmd
	}

	cmd := newCmd()
	assert.NoError(t, validateOperationSourceFlags(cmd, "", ""), "neither flag given: fine, None scope")

	cmd = newCmd()
	require.NoError(t, cmd.Flags().Set(FlagComponent, "myapp/mycomp"))
	assert.NoError(t, validateOperationSourceFlags(cmd, "myapp/mycomp", ""))

	cmd = newCmd()
	require.NoError(t, cmd.Flags().Set(FlagComponent, ""))
	assert.ErrorContains(t, validateOperationSourceFlags(cmd, "", ""), "--component must not be empty")

	cmd = newCmd()
	require.NoError(t, cmd.Flags().Set(FlagApplication, ""))
	assert.ErrorContains(t, validateOperationSourceFlags(cmd, "", ""), "--application must not be empty")
}

func TestSplitComponentRef(t *testing.T) {
	app, name, err := splitComponentRef("myapp/mycomp")
	require.NoError(t, err)
	assert.Equal(t, "myapp", app)
	assert.Equal(t, "mycomp", name)

	for _, bad := range []string{"", "myapp", "/mycomp", "myapp/"} {
		_, _, err := splitComponentRef(bad)
		assert.Error(t, err, "expected error for %q", bad)
	}
}

func TestParseOperationParams(t *testing.T) {
	params, err := parseOperationParams(nil)
	require.NoError(t, err)
	assert.Nil(t, params)

	params, err = parseOperationParams([]string{"force=true", "replicas=3"})
	require.NoError(t, err)
	assert.Equal(t, "true", params["force"])
	assert.Equal(t, "3", params["replicas"])

	_, err = parseOperationParams([]string{"noequalsign"})
	assert.Error(t, err)
}

func newOperationTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, v1beta1.SchemeBuilder.AddToScheme(scheme))
	require.NoError(t, v2alpha1.AddToScheme(scheme))
	return scheme
}

func TestResolveComponentType(t *testing.T) {
	scheme := newOperationTestScheme(t)
	app := &v1beta1.Application{
		ObjectMeta: metav1.ObjectMeta{Name: "myapp", Namespace: "default"},
		Spec: v1beta1.ApplicationSpec{
			Components: []common.ApplicationComponent{
				{Name: "mycomp", Type: "webservice"},
			},
		},
	}
	cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(app).Build()

	typ, err := resolveComponentType(context.Background(), cli, "default", "myapp", "mycomp")
	require.NoError(t, err)
	assert.Equal(t, "webservice", typ)

	_, err = resolveComponentType(context.Background(), cli, "default", "myapp", "missing")
	assert.Error(t, err)
}

func TestListAllowedOperationTemplates(t *testing.T) {
	scheme := newOperationTestScheme(t)
	restart := &v2alpha1.OperationTemplate{
		ObjectMeta: metav1.ObjectMeta{Name: "restart", Namespace: "default"},
		Spec: v2alpha1.OperationTemplateSpec{
			Description: "restart the component",
			Attach:      v2alpha1.OperationAttach{Scope: v2alpha1.OperationAttachScopeComponent},
		},
	}
	scaleWebOnly := &v2alpha1.OperationTemplate{
		ObjectMeta: metav1.ObjectMeta{Name: "scale", Namespace: systemDefinitionNamespace},
		Spec: v2alpha1.OperationTemplateSpec{
			Attach: v2alpha1.OperationAttach{
				Scope:                 v2alpha1.OperationAttachScopeComponent,
				AllowedComponentTypes: []string{"webservice"},
			},
		},
	}
	dup := &v2alpha1.OperationTemplate{
		// Same name as "restart", in vela-system: own-namespace copy wins.
		ObjectMeta: metav1.ObjectMeta{Name: "restart", Namespace: systemDefinitionNamespace},
		Spec: v2alpha1.OperationTemplateSpec{
			Description: "should be shadowed",
			Attach:      v2alpha1.OperationAttach{Scope: v2alpha1.OperationAttachScopeComponent},
		},
	}
	cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(restart, scaleWebOnly, dup).Build()

	templates, err := listAllowedOperationTemplates(context.Background(), cli, "default", operationListModeComponent, "webservice", nil)
	require.NoError(t, err)
	byName := map[string]v2alpha1.OperationTemplate{}
	for _, tmpl := range templates {
		byName[tmpl.Name] = tmpl
	}
	require.Contains(t, byName, "restart")
	require.Contains(t, byName, "scale")
	assert.Equal(t, "restart the component", byName["restart"].Spec.Description)

	templates, err = listAllowedOperationTemplates(context.Background(), cli, "default", operationListModeComponent, "worker", nil)
	require.NoError(t, err)
	byName = map[string]v2alpha1.OperationTemplate{}
	for _, tmpl := range templates {
		byName[tmpl.Name] = tmpl
	}
	assert.Contains(t, byName, "restart")
	assert.NotContains(t, byName, "scale", "scale is restricted to webservice")
}

func TestListAllowedOperationTemplatesApplicationAndNoneScope(t *testing.T) {
	scheme := newOperationTestScheme(t)
	appTmpl := &v2alpha1.OperationTemplate{
		ObjectMeta: metav1.ObjectMeta{Name: "patch-app", Namespace: "default"},
		Spec:       v2alpha1.OperationTemplateSpec{Attach: v2alpha1.OperationAttach{Scope: v2alpha1.OperationAttachScopeApplication}},
	}
	noneTmpl := &v2alpha1.OperationTemplate{
		ObjectMeta: metav1.ObjectMeta{Name: "notify", Namespace: "default"},
		Spec:       v2alpha1.OperationTemplateSpec{Attach: v2alpha1.OperationAttach{Scope: v2alpha1.OperationAttachScopeNone}},
	}
	cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(appTmpl, noneTmpl).Build()

	templates, err := listAllowedOperationTemplates(context.Background(), cli, "default", operationListModeApplication, "", &v1beta1.Application{})
	require.NoError(t, err)
	require.Len(t, templates, 1)
	assert.Equal(t, "patch-app", templates[0].Name)

	templates, err = listAllowedOperationTemplates(context.Background(), cli, "default", operationListModeNone, "", nil)
	require.NoError(t, err)
	require.Len(t, templates, 1)
	assert.Equal(t, "notify", templates[0].Name)
}

func TestListAllowedOperationTemplatesExcludesInvalidForScope(t *testing.T) {
	scheme := newOperationTestScheme(t)
	componentWithSelector := &v2alpha1.OperationTemplate{
		ObjectMeta: metav1.ObjectMeta{Name: "component-with-selector", Namespace: "default"},
		Spec: v2alpha1.OperationTemplateSpec{Attach: v2alpha1.OperationAttach{
			Scope:    v2alpha1.OperationAttachScopeComponent,
			Selector: &v2alpha1.OperationApplicationSelector{MatchLabels: map[string]string{"env": "prod"}},
		}},
	}
	applicationWithAllowedTypes := &v2alpha1.OperationTemplate{
		ObjectMeta: metav1.ObjectMeta{Name: "application-with-allowed-types", Namespace: "default"},
		Spec: v2alpha1.OperationTemplateSpec{Attach: v2alpha1.OperationAttach{
			Scope:                 v2alpha1.OperationAttachScopeApplication,
			AllowedComponentTypes: []string{"webservice"},
		}},
	}
	noneWithAllowedTypes := &v2alpha1.OperationTemplate{
		ObjectMeta: metav1.ObjectMeta{Name: "none-with-allowed-types", Namespace: "default"},
		Spec: v2alpha1.OperationTemplateSpec{Attach: v2alpha1.OperationAttach{
			Scope:                 v2alpha1.OperationAttachScopeNone,
			AllowedComponentTypes: []string{"webservice"},
		}},
	}
	cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(componentWithSelector, applicationWithAllowedTypes, noneWithAllowedTypes).Build()

	templates, err := listAllowedOperationTemplates(context.Background(), cli, "default", operationListModeComponent, "webservice", nil)
	require.NoError(t, err)
	assert.Empty(t, templates, "a Component-scoped template carrying a selector would be rejected by the controller, so it must not be offered")

	templates, err = listAllowedOperationTemplates(context.Background(), cli, "default", operationListModeApplication, "", &v1beta1.Application{})
	require.NoError(t, err)
	assert.Empty(t, templates, "an Application-scoped template carrying allowedComponentTypes would be rejected by the controller, so it must not be offered")

	templates, err = listAllowedOperationTemplates(context.Background(), cli, "default", operationListModeNone, "", nil)
	require.NoError(t, err)
	assert.Empty(t, templates, "a None-scoped template carrying allowedComponentTypes would be rejected by the controller, so it must not be offered")
}

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

package step

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	wfTypesv1alpha1 "github.com/kubevela/pkg/apis/oam/v1alpha1"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
	common2 "github.com/oam-dev/kubevela/pkg/utils/common"
)

func sharedWorkflow(namespace, name, step string) *wfTypesv1alpha1.Workflow {
	return &wfTypesv1alpha1.Workflow{
		ObjectMeta: v1.ObjectMeta{Name: name, Namespace: namespace},
		WorkflowSpec: wfTypesv1alpha1.WorkflowSpec{
			Steps: []wfTypesv1alpha1.WorkflowStep{{WorkflowStepBase: wfTypesv1alpha1.WorkflowStepBase{Name: step, Type: "suspend"}}},
		},
	}
}

func TestGetRefWorkflow(t *testing.T) {
	ctx := context.Background()
	cli := func(objs ...client.Object) client.Client {
		return fake.NewClientBuilder().WithScheme(common2.Scheme).WithObjects(objs...).Build()
	}

	t.Run("a Workflow in the application's namespace", func(t *testing.T) {
		wf, err := GetRefWorkflow(ctx, cli(sharedWorkflow("shop", "release", "local")), "shop", "release")
		require.NoError(t, err)
		require.Equal(t, "local", wf.Steps[0].Name)
	})

	t.Run("falls back to the system namespace, as definitions do", func(t *testing.T) {
		wf, err := GetRefWorkflow(ctx, cli(sharedWorkflow(oam.SystemDefinitionNamespace, "release", "global")), "shop", "release")
		require.NoError(t, err)
		require.Equal(t, "global", wf.Steps[0].Name)
		require.Equal(t, oam.SystemDefinitionNamespace, wf.Namespace)
	})

	t.Run("a Workflow in the application's namespace wins over the system one", func(t *testing.T) {
		wf, err := GetRefWorkflow(ctx, cli(
			sharedWorkflow("shop", "release", "local"),
			sharedWorkflow(oam.SystemDefinitionNamespace, "release", "global"),
		), "shop", "release")
		require.NoError(t, err)
		require.Equal(t, "local", wf.Steps[0].Name)
	})

	t.Run("not found in either", func(t *testing.T) {
		_, err := GetRefWorkflow(ctx, cli(), "shop", "release")
		require.True(t, apierrors.IsNotFound(err))
	})
}

func TestRefWorkflowStepGeneratorUsesSystemWorkflow(t *testing.T) {
	c := fake.NewClientBuilder().WithScheme(common2.Scheme).WithObjects(sharedWorkflow(oam.SystemDefinitionNamespace, "release", "global")).Build()
	app := appWithRef("shop", "release")
	steps, err := (&RefWorkflowStepGenerator{Context: context.Background(), Client: c}).Generate(app, nil)
	require.NoError(t, err)
	require.Equal(t, "global", steps[0].Name)
}

func appWithRef(namespace, ref string) *v1beta1.Application {
	return &v1beta1.Application{
		ObjectMeta: v1.ObjectMeta{Name: "app", Namespace: namespace},
		Spec:       v1beta1.ApplicationSpec{Workflow: &v1beta1.Workflow{Ref: ref}},
	}
}

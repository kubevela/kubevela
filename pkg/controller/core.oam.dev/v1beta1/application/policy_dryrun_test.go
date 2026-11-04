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

package application

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	common2 "github.com/oam-dev/kubevela/pkg/controller/common"
	"github.com/oam-dev/kubevela/pkg/oam"
)

func dryRunGlobal(name, namespace string) *v1beta1.PolicyDefinition {
	return &v1beta1.PolicyDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec: v1beta1.PolicyDefinitionSpec{Scope: v1beta1.ApplicationScope, Global: true,
			Schematic: &common.Schematic{CUE: &common.CUE{Template: `parameter: {}, output: labels: {"` + name + `": "set"}`}}},
	}
}

func TestSimulatePolicyApplicationWritesNothing(t *testing.T) {
	useTestPolicyScopeIndex(t)
	global := dryRunGlobal("sys-global", oam.SystemDefinitionNamespace)
	app := &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "default"}}
	cli := fake.NewClientBuilder().WithScheme(newTestScheme(t)).WithObjects(global, app.DeepCopy()).Build()

	result, err := SimulatePolicyApplication(context.Background(), cli, app)
	require.NoError(t, err)
	require.Empty(t, result.Errors)
	require.Equal(t, "set", result.Application.Labels["sys-global"], "the simulation applies the global")

	cms := &corev1.ConfigMapList{}
	require.NoError(t, cli.List(context.Background(), cms, client.InNamespace("default")))
	require.Empty(t, cms.Items, "a dry-run writes no policy ConfigMap")
	stored := &v1beta1.Application{}
	require.NoError(t, cli.Get(context.Background(), client.ObjectKeyFromObject(app), stored))
	require.Empty(t, stored.Labels, "a dry-run leaves the Application alone")
}

func TestSimulatePolicyApplicationReportsControllerFlags(t *testing.T) {
	useTestPolicyScopeIndex(t)
	nsGlobal := dryRunGlobal("ns-global", "default")
	app := &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "default"}}
	cli := fake.NewClientBuilder().WithScheme(newTestScheme(t)).WithObjects(nsGlobal).Build()

	result, err := SimulatePolicyApplication(context.Background(), cli, app.DeepCopy())
	require.NoError(t, err)
	require.Equal(t, PolicyDryRunFlags{AllowNamespaceGlobalPolicies: true, AllowSkipGlobalPolicies: true}, result.Flags)
	require.Equal(t, "set", result.Application.Labels["ns-global"], "a namespace global applies by default")

	orig := common2.AllowNamespaceGlobalPolicies
	t.Cleanup(func() { common2.AllowNamespaceGlobalPolicies = orig })
	common2.AllowNamespaceGlobalPolicies = false
	result, err = SimulatePolicyApplication(context.Background(), cli, app.DeepCopy())
	require.NoError(t, err)
	require.Equal(t, PolicyDryRunFlags{AllowSkipGlobalPolicies: true}, result.Flags)
	require.NotContains(t, result.Application.Labels, "ns-global", "restricted, a namespace global does not apply")
}

func TestSimulatePolicyApplicationReportsHeldData(t *testing.T) {
	useTestPolicyScopeIndex(t)
	setRolloutDefaults(t, false, false, false)
	global := dryRunGlobal("sys-global", oam.SystemDefinitionNamespace)
	app := &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "default"}}
	app.Status.ApplicationPoliciesConfigMap = policyConfigMapName(app.Namespace, app.Name)
	app.Status.LatestRevision = &common.Revision{Name: "app-v1", Revision: 1}
	rev := &v1beta1.ApplicationRevision{ObjectMeta: metav1.ObjectMeta{Name: "app-v1", Namespace: app.Namespace}}
	rev.Spec.Application = *app.DeepCopy()
	rev.Spec.PolicyDefinitions = map[string]v1beta1.PolicyDefinition{global.Name: *global.DeepCopy()}
	recorded := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: app.Status.ApplicationPoliciesConfigMap, Namespace: app.Namespace},
		Data: map[string]string{policyAppliedOutputsKey: encodeAppliedOutputs(map[string]appliedPolicyOutput{
			"global:sys-global": {Labels: map[string]string{"sys-global": "applied"}},
		})},
	}
	cli := fake.NewClientBuilder().WithScheme(newTestScheme(t)).WithObjects(global, app.DeepCopy(), rev, recorded).Build()

	result, err := SimulatePolicyApplication(context.Background(), cli, app)
	require.NoError(t, err)
	require.Empty(t, result.Errors)
	require.Equal(t, "applied", result.Application.Labels["sys-global"], "apply-data-changes off keeps the applied label")
	output, _ := result.PolicyDetails["sys-global"]["output"].(map[string]interface{})
	require.Equal(t, map[string]string{"sys-global": "applied"}, output["labels"], "the details show what the controller applies")
}

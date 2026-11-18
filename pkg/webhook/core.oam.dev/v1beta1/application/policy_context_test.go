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
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	featuregatetesting "k8s.io/component-base/featuregate/testing"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/appfile"
	common2 "github.com/oam-dev/kubevela/pkg/controller/common"
	"github.com/oam-dev/kubevela/pkg/features"
	"github.com/oam-dev/kubevela/pkg/oam"
	oamutil "github.com/oam-dev/kubevela/pkg/oam/util"
)

const tenantNS = "team-a"

// reader is a ComponentDefinition labelling a ConfigMap from context.custom.
func reader(name, labels string) *v1beta1.ComponentDefinition {
	imports := ""
	if strings.Contains(labels, "strings.") {
		imports = "import \"strings\"\n"
	}
	return &v1beta1.ComponentDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: oam.SystemDefinitionNamespace},
		Spec: v1beta1.ComponentDefinitionSpec{
			Workload:  common.WorkloadTypeDescriptor{Definition: common.WorkloadGVK{APIVersion: "v1", Kind: "ConfigMap"}},
			Schematic: &common.Schematic{CUE: &common.CUE{Template: imports + "parameter: {}\noutput: {apiVersion: \"v1\", kind: \"ConfigMap\", metadata: {name: context.name, labels: {" + labels + "}}}"}},
		},
	}
}

func appPolicyDef(namespace, name, template string, global bool, priority int32) *v1beta1.PolicyDefinition {
	return &v1beta1.PolicyDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec: v1beta1.PolicyDefinitionSpec{
			Global:    global,
			Scope:     v1beta1.ApplicationScope,
			Priority:  priority,
			Schematic: &common.Schematic{CUE: &common.CUE{Template: template}},
		},
	}
}

const (
	teamString = "schema: ctx: close({team: string, mesh?: string})\noutput: ctx: team: \"payments\""
	teamInt    = "schema: ctx: close({team: int})\noutput: ctx: team: 1"
)

func ctxApp(compType string, annotations map[string]string, policies ...v1beta1.AppPolicy) *v1beta1.Application {
	return &v1beta1.Application{
		ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: tenantNS, Annotations: annotations},
		Spec: v1beta1.ApplicationSpec{
			Components: []common.ApplicationComponent{{Name: "api", Type: compType, Properties: &runtime.RawExtension{Raw: []byte(`{}`)}}},
			Policies:   policies,
		},
	}
}

func policyRef(name, typ string) v1beta1.AppPolicy {
	return v1beta1.AppPolicy{Name: name, Type: typ, Properties: &runtime.RawExtension{Raw: []byte(`{}`)}}
}

func ctxHandler(objs ...client.Object) *ValidatingHandler {
	scheme := runtime.NewScheme()
	_ = v1beta1.AddToScheme(scheme)
	return &ValidatingHandler{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()}
}

func policyGates(t *testing.T, appScoped, global bool) {
	featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.EnableApplicationScopedPolicies, appScoped)
	featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.EnableGlobalPolicies, global)
}

func setFlag(t *testing.T, flag *bool, v bool) {
	old := *flag
	*flag = v
	t.Cleanup(func() { *flag = old })
}

func componentErrors(h *ValidatingHandler, app *v1beta1.Application) string {
	var msgs []string
	// As Handle does before validating.
	ctx := oamutil.SetNamespaceInCtx(context.Background(), app.Namespace)
	for _, e := range h.ValidateComponents(ctx, app) {
		msgs = append(msgs, e.Error())
	}
	return strings.Join(msgs, "\n")
}

func TestPolicyContextAtAdmission(t *testing.T) {
	readsTeam := reader("reads-team", "team: context.custom.team")
	lowersTeam := reader("lowers-team", "team: strings.ToLower(context.custom.team)")
	readsMesh := reader("reads-mesh", "mesh: context.custom.mesh")
	cases := map[string]struct {
		objs        []client.Object
		app         *v1beta1.Application
		appScoped   bool
		global      bool
		allowNSGlob bool
		allowSkip   bool
		// wantErr is a fragment of the error, or empty when admitted.
		wantErr string
	}{
		"without Application-scoped policies the context is not judged": {
			objs: []client.Object{readsTeam},
			app:  ctxApp("reads-team", nil),
		},
		"no policy declares a key the component reads": {
			objs:      []client.Object{readsTeam},
			app:       ctxApp("reads-team", nil),
			appScoped: true, global: true,
			wantErr: "no applicable policy declares it",
		},
		"a vela-system global declares it": {
			objs:      []client.Object{readsTeam, appPolicyDef(oam.SystemDefinitionNamespace, "identity", teamString, true, 0)},
			app:       ctxApp("reads-team", nil),
			appScoped: true, global: true,
		},
		"an optional key read without a guard": {
			objs:      []client.Object{readsMesh, appPolicyDef(oam.SystemDefinitionNamespace, "identity", teamString, true, 0)},
			app:       ctxApp("reads-mesh", nil),
			appScoped: true, global: true,
			wantErr: `context.custom.mesh is optional in policy "identity"`,
		},
		"a namespace global is not counted unless namespace globals are allowed": {
			objs:      []client.Object{readsTeam, appPolicyDef(tenantNS, "identity", teamString, true, 0)},
			app:       ctxApp("reads-team", nil),
			appScoped: true, global: true,
			wantErr: "no applicable policy declares it",
		},
		"a namespace global counts when allowed": {
			objs:      []client.Object{readsTeam, appPolicyDef(tenantNS, "identity", teamString, true, 0)},
			app:       ctxApp("reads-team", nil),
			appScoped: true, global: true, allowNSGlob: true,
		},
		"a namespace global replaces the vela-system one it shares a name with": {
			objs: []client.Object{lowersTeam,
				appPolicyDef(oam.SystemDefinitionNamespace, "identity", teamInt, true, 0),
				appPolicyDef(tenantNS, "identity", teamString, true, 0)},
			app:       ctxApp("lowers-team", nil),
			appScoped: true, global: true, allowNSGlob: true,
		},
		"globals run by name, wherever they live": {
			objs: []client.Object{lowersTeam,
				appPolicyDef(tenantNS, "zeta", teamString, true, 0),
				appPolicyDef(oam.SystemDefinitionNamespace, "alpha", teamInt, true, 0)},
			app:       ctxApp("lowers-team", nil),
			appScoped: true, global: true, allowNSGlob: true,
		},
		"a vela-system policy that is not global is not counted as one": {
			objs:      []client.Object{readsTeam, appPolicyDef(oam.SystemDefinitionNamespace, "identity", teamString, false, 0)},
			app:       ctxApp("reads-team", nil),
			appScoped: true, global: true,
			wantErr: "no applicable policy declares it",
		},
		"a global that is not Application-scoped is not counted": {
			objs: []client.Object{readsTeam, func() client.Object {
				pd := appPolicyDef(oam.SystemDefinitionNamespace, "identity", teamString, true, 0)
				pd.Spec.Scope = ""
				return pd
			}()},
			app:       ctxApp("reads-team", nil),
			appScoped: true, global: true,
			wantErr: "no applicable policy declares it",
		},
		"globals are not counted when the gate is off": {
			objs:      []client.Object{readsTeam, appPolicyDef(oam.SystemDefinitionNamespace, "identity", teamString, true, 0)},
			app:       ctxApp("reads-team", nil),
			appScoped: true,
			wantErr:   "no applicable policy declares it",
		},
		"a skip the controller refuses leaves the globals counted": {
			objs:      []client.Object{readsTeam, appPolicyDef(oam.SystemDefinitionNamespace, "identity", teamString, true, 0)},
			app:       ctxApp("reads-team", map[string]string{oam.AnnotationSkipGlobalPolicies: "true"}),
			appScoped: true, global: true,
		},
		"a skip the controller permits leaves them out": {
			objs:      []client.Object{readsTeam, appPolicyDef(oam.SystemDefinitionNamespace, "identity", teamString, true, 0)},
			app:       ctxApp("reads-team", map[string]string{oam.AnnotationSkipGlobalPolicies: "true"}),
			appScoped: true, global: true, allowSkip: true,
			wantErr: "no applicable policy declares it",
		},
		"an explicit policy declares it": {
			objs:      []client.Object{readsTeam, appPolicyDef(tenantNS, "identity", teamString, false, 0)},
			app:       ctxApp("reads-team", nil, policyRef("id", "identity")),
			appScoped: true, global: true,
		},
		"an explicit policy runs after a global of the same priority": {
			objs: []client.Object{lowersTeam,
				appPolicyDef(oam.SystemDefinitionNamespace, "platform", teamInt, true, 0),
				appPolicyDef(tenantNS, "identity", teamString, false, 0)},
			app:       ctxApp("lowers-team", nil, policyRef("id", "identity")),
			appScoped: true, global: true,
		},
		"a global with a later priority runs after an explicit policy": {
			objs: []client.Object{lowersTeam,
				appPolicyDef(oam.SystemDefinitionNamespace, "platform", teamInt, true, 1),
				appPolicyDef(tenantNS, "identity", teamString, false, 0)},
			app:       ctxApp("lowers-team", nil, policyRef("id", "identity")),
			appScoped: true, global: true,
			wantErr: `uses context.custom.team against its type in policy "platform"`,
		},
		"a pinned explicit policy is judged by its revision": {
			objs: []client.Object{lowersTeam,
				appPolicyDef(tenantNS, "identity", teamString, false, 0),
				policyRevision(tenantNS, "identity", 1, teamInt)},
			app:       ctxApp("lowers-team", nil, policyRef("id", "identity@v1")),
			appScoped: true, global: true,
			wantErr: `uses context.custom.team against its type in policy "identity"`,
		},
		"an explicit policy that is not Application-scoped supplies nothing": {
			objs: []client.Object{readsTeam, &v1beta1.PolicyDefinition{
				ObjectMeta: metav1.ObjectMeta{Name: "plain-policy", Namespace: oam.SystemDefinitionNamespace},
				Spec:       v1beta1.PolicyDefinitionSpec{Schematic: &common.Schematic{CUE: &common.CUE{Template: `output: ctx: team: "x"`}}},
			}},
			app:       ctxApp("reads-team", nil, policyRef("t", "plain-policy")),
			appScoped: true, global: true,
			wantErr: "no applicable policy declares it",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			policyGates(t, tc.appScoped, tc.global)
			setFlag(t, &common2.AllowNamespaceGlobalPolicies, tc.allowNSGlob)
			setFlag(t, &common2.AllowSkipGlobalPolicies, tc.allowSkip)
			got := componentErrors(ctxHandler(tc.objs...), tc.app)
			if tc.wantErr == "" {
				require.Empty(t, got)
				return
			}
			require.Contains(t, got, tc.wantErr)
		})
	}
}

func policyRevision(namespace, name string, revision int64, template string) *v1beta1.DefinitionRevision {
	pd := appPolicyDef(namespace, name, template, false, 0)
	return &v1beta1.DefinitionRevision{
		ObjectMeta: metav1.ObjectMeta{Name: name + "-v1", Namespace: namespace},
		Spec: v1beta1.DefinitionRevisionSpec{
			Revision:         revision,
			DefinitionType:   common.PolicyType,
			PolicyDefinition: *pd,
		},
	}
}

// listFailingClient fails every List, as a transient API error would.
type listFailingClient struct {
	client.Client
	lists int
}

func (c *listFailingClient) List(context.Context, client.ObjectList, ...client.ListOption) error {
	c.lists++
	return apierrors.NewServiceUnavailable("apiserver is having a moment")
}

// The policies are looked up only for an Application that reads their
// context, and a failed lookup does not refuse it: the render still does.
func TestPolicyContextLookups(t *testing.T) {
	policyGates(t, true, true)
	scheme := runtime.NewScheme()
	_ = v1beta1.AddToScheme(scheme)
	build := func(objs ...client.Object) *listFailingClient {
		return &listFailingClient{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()}
	}

	t.Run("an Application reading no context looks nothing up", func(t *testing.T) {
		c := build(reader("plain", "a: \"b\""))
		require.Empty(t, componentErrors(&ValidatingHandler{Client: c}, ctxApp("plain", nil)))
		require.Zero(t, c.lists)
	})
	t.Run("a failed lookup skips the check", func(t *testing.T) {
		c := build(reader("reads-team", "team: context.custom.team"))
		require.Empty(t, componentErrors(&ValidatingHandler{Client: c}, ctxApp("reads-team", nil)))
		require.NotZero(t, c.lists)
	})
}

// getFailingClient fails every Get of a PolicyDefinition, as a transient API
// error would.
type getFailingClient struct{ client.Client }

func (c getFailingClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if _, ok := obj.(*v1beta1.PolicyDefinition); ok {
		return apierrors.NewServiceUnavailable("apiserver is having a moment")
	}
	return c.Client.Get(ctx, key, obj, opts...)
}

// An explicit policy whose definition cannot be read may supply anything: the
// view is not complete, so a key it might declare is not refused.
func TestPolicyContextViewUnknownExplicitPolicy(t *testing.T) {
	policyGates(t, true, true)
	h := &ValidatingHandler{Client: getFailingClient{ctxHandler().Client}}
	af := &appfile.Appfile{ParsedComponents: []*appfile.Component{{FullTemplate: &appfile.Template{TemplateStr: "x: context.custom.team"}}}}

	view := h.policyContextView(context.Background(), ctxApp("c", nil, policyRef("id", "identity")), af)
	require.NotNil(t, view)
	require.False(t, view.Complete)

	view = h.policyContextView(context.Background(), ctxApp("c", nil), af)
	require.NotNil(t, view)
	require.True(t, view.Complete, "with no policies the view is complete, and empty")
}

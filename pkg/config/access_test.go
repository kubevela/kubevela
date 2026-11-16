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

package config

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	authorizationv1 "k8s.io/api/authorization/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/apiserver/pkg/endpoints/request"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	featuregatetesting "k8s.io/component-base/featuregate/testing"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	apitypes "github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/features"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/utils/common"
)

func TestAccessToConfig(t *testing.T) {
	sys := apitypes.DefaultKubeVelaNS
	for _, c := range []struct {
		name                  string
		verb, namespace, from string
		shared, wantReview    bool
	}{
		{"own namespace", "get", "team-a", "team-a", false, false},
		{"own namespace write", "delete", "team-a", "team-a", false, false},
		{"another tenant", "get", "team-b", "team-a", false, true},
		{"a shared system config is read from anywhere", "get", sys, "team-a", true, false},
		{"a shared system config is listed from anywhere", "list", sys, "team-a", true, false},
		{"a system config that is not shared", "get", sys, "team-a", false, true},
		{"a shared system config is not written from elsewhere", "create", sys, "team-a", true, true},
		{"a shared system config is not deleted from elsewhere", "delete", sys, "team-a", true, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			access := AccessToConfig(c.verb, c.namespace, "db", c.from, c.shared)
			require.Equal(t, c.wantReview, access.Review != nil)
			if c.wantReview {
				require.Equal(t, c.verb, access.Review.Verb)
				require.Equal(t, c.namespace, access.Review.Namespace)
				require.Equal(t, "secrets", access.Review.Resource)
				require.NotEmpty(t, access.Reason)
			}
		})
	}
}

// reviewer answers access reviews with allowed and records what it was asked.
type reviewer struct {
	client.Client
	allowed bool
	asked   []authorizationv1.ResourceAttributes
}

func (r *reviewer) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	if sar, ok := obj.(*authorizationv1.SubjectAccessReview); ok {
		r.asked = append(r.asked, *sar.Spec.ResourceAttributes)
		sar.Status.Allowed = r.allowed
		return nil
	}
	return r.Client.Create(ctx, obj, opts...)
}

func configsIn(objs ...client.Object) client.Client {
	sys := apitypes.DefaultKubeVelaNS
	scoped := func(name, scope string) client.Object {
		return &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: sys,
			Labels: map[string]string{apitypes.LabelConfigScope: scope, apitypes.LabelConfigCatalog: apitypes.VelaCoreConfig}}}
	}
	notAConfig := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "platform-token", Namespace: sys,
		Labels: map[string]string{apitypes.LabelConfigScope: ScopeSystem}}}
	template := func(name, scope string) client.Object {
		return &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: TemplateConfigMapNamePrefix + name, Namespace: sys,
			Labels:      map[string]string{apitypes.LabelConfigScope: scope},
			Annotations: map[string]string{apitypes.AnnotationConfigAlias: name}}}
	}
	objs = append(objs, notAConfig, scoped("registry", ScopeSystem), scoped("tenant-db", "project"),
		template("registry", ScopeSystem), template("tenant-db", "project"))
	return fake.NewClientBuilder().WithScheme(common.Scheme).WithObjects(objs...).Build()
}

func TestSharedSystemConfig(t *testing.T) {
	cli := configsIn()
	for name, want := range map[string]bool{"registry": true, "tenant-db": false, "missing": false,
		"platform-token": false} { // a Secret with the scope label that is not a KubeVela config
		got, err := SharedSystemConfig(context.Background(), cli, name)
		require.NoError(t, err, name)
		require.Equal(t, want, got, name)
	}
}

func TestAllowConfigAccess(t *testing.T) {
	sys := apitypes.DefaultKubeVelaNS
	ctx := context.Background()
	cli := configsIn()

	require.NoError(t, AllowConfigAccess(ctx, cli, "delete", "team-a", "db", "", "team-a"), "its own namespace")
	require.NoError(t, AllowConfigAccess(ctx, cli, "get", sys, "registry", "", "team-a"), "a shared system config")
	require.NoError(t, AllowConfigAccess(ctx, cli, "list", sys, "", "registry", "team-a"), "a system template's configs")

	err := AllowConfigAccess(ctx, cli, "get", "team-b", "db", "", "team-a")
	require.ErrorContains(t, err, "config team-b/db is outside the application's namespace team-a")
	require.ErrorContains(t, err, "feature gate is off")
	require.Error(t, AllowConfigAccess(ctx, cli, "get", sys, "tenant-db", "", "team-a"), "a system config that is not shared")
	require.Error(t, AllowConfigAccess(ctx, cli, "list", sys, "", "tenant-db", "team-a"), "a template that is not system-scoped")
	require.Error(t, AllowConfigAccess(ctx, cli, "delete", sys, "registry", "", "team-a"), "shared is read-only")

	alice := request.WithUser(ctx, &user.DefaultInfo{Name: "alice", Groups: []string{"team-a"}})
	require.ErrorContains(t, AllowConfigAccess(alice, &reviewer{Client: cli, allowed: true}, "get", "team-b", "db", "", "team-a"), "feature gate is off",
		"with the gate off the user is the Application's own annotation, so it is not reviewed")
	featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultMutableFeatureGate, features.AuthenticateApplication, true)
	denying := &reviewer{Client: cli}
	require.ErrorContains(t, AllowConfigAccess(alice, denying, "get", "team-b", "db", "", "team-a"), "alice may not")
	require.Equal(t, []authorizationv1.ResourceAttributes{{Verb: "get", Version: "v1", Resource: "secrets", Namespace: "team-b", Name: "db"}}, denying.asked)
	require.NoError(t, AllowConfigAccess(alice, &reviewer{Client: cli, allowed: true}, "get", "team-b", "db", "", "team-a"))
}

func TestStepListScope(t *testing.T) {
	sys := apitypes.DefaultKubeVelaNS
	ctx := context.Background()
	cli := configsIn()
	from := map[string]string{oam.LabelAppNamespace: "team-a"}
	for _, c := range []struct {
		why                 string
		labels              map[string]string
		namespace, template string
		want                string
	}{
		{"a system template's configs, from another namespace", from, sys, "registry", ScopeSystem},
		{"a template that is not system-scoped", from, sys, "tenant-db", ""},
		{"the Application's own namespace", from, "team-a", "registry", ""},
		{"no Application", nil, sys, "registry", ""},
	} {
		got, err := StepListScope(ctx, cli, c.labels, c.namespace, c.template)
		require.NoError(t, err, c.why)
		require.Equal(t, c.want, got, c.why)
	}
}

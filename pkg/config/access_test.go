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
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/apiserver/pkg/endpoints/request"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	featuregatetesting "k8s.io/component-base/featuregate/testing"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	configv1alpha1 "github.com/oam-dev/kubevela/apis/config.oam.dev/v1alpha1"
	apitypes "github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/features"
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
				require.NotEmpty(t, access.Reason)
			}
		})
	}
}

func TestSharedSystemConfig(t *testing.T) {
	sys := apitypes.DefaultKubeVelaNS
	template := func(name string, scope configv1alpha1.ConfigTemplateScope) client.Object {
		return &configv1alpha1.ConfigTemplate{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: sys},
			Spec:       configv1alpha1.ConfigTemplateSpec{Scope: scope, Template: "template: parameter: {}"},
		}
	}
	cfg := func(name, tmpl string) client.Object {
		return &configv1alpha1.Config{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: sys},
			Spec:       configv1alpha1.ConfigSpec{TemplateRef: &configv1alpha1.ConfigTemplateReference{Name: tmpl}},
		}
	}
	legacy := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "legacy-registry", Namespace: sys,
		Labels: map[string]string{apitypes.LabelConfigType: "registry"}}}
	cli := fake.NewClientBuilder().WithScheme(common.Scheme).WithObjects(
		template("registry", configv1alpha1.ConfigTemplateScopeSystem),
		template("tenant-db", configv1alpha1.ConfigTemplateScopeNamespace),
		cfg("registry", "registry"), cfg("tenant-db", "tenant-db"), legacy,
	).Build()

	for name, want := range map[string]bool{
		"registry":        true,
		"tenant-db":       false,
		"legacy-registry": true,
		"missing":         false,
	} {
		got, err := SharedSystemConfig(context.Background(), cli, name)
		require.NoError(t, err, name)
		require.Equal(t, want, got, name)
	}
}

func TestAllowConfigAccessAsksAboutWhatHoldsTheConfig(t *testing.T) {
	featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultMutableFeatureGate, features.AuthenticateApplication, true)
	asAlice := request.WithUser(context.Background(), &user.DefaultInfo{Name: "alice"})
	reviewed := func(scheme *runtime.Scheme, mapper meta.RESTMapper) authorizationv1.ResourceAttributes {
		var got authorizationv1.ResourceAttributes
		cli := fake.NewClientBuilder().WithScheme(scheme).WithRESTMapper(mapper).WithInterceptorFuncs(interceptor.Funcs{
			Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
				if sar, ok := obj.(*authorizationv1.SubjectAccessReview); ok {
					got = *sar.Spec.ResourceAttributes
					sar.Status.Allowed = true
					return nil
				}
				return c.Create(ctx, obj, opts...)
			},
		}).Build()
		require.NoError(t, AllowConfigAccess(asAlice, cli, "create", "team-b", "db", "", "team-a"))
		return got
	}

	withConfig := meta.NewDefaultRESTMapper(nil)
	withConfig.Add(configv1alpha1.ConfigGroupVersionKind, meta.RESTScopeNamespace)
	served := reviewed(common.Scheme, withConfig)
	require.Equal(t, "config.oam.dev", served.Group)
	require.Equal(t, "configs", served.Resource)

	legacy := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(legacy))
	unserved := reviewed(legacy, meta.NewDefaultRESTMapper(nil))
	require.Equal(t, authorizationv1.ResourceAttributes{Verb: "create", Version: "v1", Resource: "secrets", Namespace: "team-b", Name: "db"}, unserved)
}

// With the gate off the controller acts as itself and the user is only the Application's own
// annotation, so a read outside its namespace is refused rather than reviewed for that name.
func TestAllowConfigAccessIgnoresTheUserWithTheGateOff(t *testing.T) {
	featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultMutableFeatureGate, features.AuthenticateApplication, false)
	asAlice := request.WithUser(context.Background(), &user.DefaultInfo{Name: "alice"})
	cli := fake.NewClientBuilder().WithScheme(common.Scheme).WithInterceptorFuncs(interceptor.Funcs{
		Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
			t.Fatalf("no review is asked for with the gate off")
			return nil
		},
	}).Build()
	require.ErrorContains(t, AllowConfigAccess(asAlice, cli, "get", "team-b", "db", "", "team-a"), "feature gate is off")
}

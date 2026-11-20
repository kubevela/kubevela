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
	admissionv1 "k8s.io/api/admission/v1"
	authenticationv1 "k8s.io/api/authentication/v1"
	authv1 "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	featuregatetesting "k8s.io/component-base/featuregate/testing"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/features"
	"github.com/oam-dev/kubevela/pkg/oam"
)

// platformAdmins answers access reviews: only the users named may create Applications in
// vela-system.
type platformAdmins struct {
	client.Client
	admins map[string]bool
}

func (p *platformAdmins) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	if sar, ok := obj.(*authv1.SubjectAccessReview); ok {
		ra := sar.Spec.ResourceAttributes
		sar.Status.Allowed = !(ra.Resource == "applications" && ra.Verb == "create" && ra.Namespace == oam.SystemDefinitionNamespace) || p.admins[sar.Spec.User]
		return nil
	}
	return p.Client.Create(ctx, obj, opts...)
}

// An addon component makes the controller install the addon, its Application in vela-system and
// whatever cluster-scoped resources it holds, so its author must be able to install addons
// themselves.
func TestAddonComponentsNeedAnAuthorWhoMayInstallAddons(t *testing.T) {
	featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultMutableFeatureGate, features.EnableAddonComponent, true)
	controller := "system:serviceaccount:vela-system:kubevela"
	scheme := runtime.NewScheme()
	require.NoError(t, v1beta1.AddToScheme(scheme))
	require.NoError(t, authv1.AddToScheme(scheme))
	h := &ValidatingHandler{controllerUsername: controller,
		Client: &platformAdmins{Client: fake.NewClientBuilder().WithScheme(scheme).Build(), admins: map[string]bool{"platform-admin": true}}}
	app := func(labels map[string]string, types ...string) *v1beta1.Application {
		a := &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "team-a", Labels: labels}}
		for _, t := range types {
			a.Spec.Components = append(a.Spec.Components, common.ApplicationComponent{Name: t + "-c", Type: t})
		}
		return a
	}
	as := func(user string) admission.Request {
		return admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{UserInfo: authenticationv1.UserInfo{Username: user}}}
	}

	errs := h.ValidateAddonInstallers(context.Background(), app(nil, "webservice", "addon"), as("tenant"))
	require.Len(t, errs, 1)
	require.Equal(t, "spec.components[1].type", errs[0].Field)
	require.Contains(t, errs[0].Detail, "create Applications in namespace \"vela-system\"")

	require.Empty(t, h.ValidateAddonInstallers(context.Background(), app(nil, "addon"), as("platform-admin")))
	require.Empty(t, h.ValidateAddonInstallers(context.Background(), app(nil, "webservice"), as("tenant")), "no addon component, nothing to check")

	fromTenant := map[string]string{oam.LabelAppName: "outer", oam.LabelAppNamespace: "team-a"}
	errs = h.ValidateAddonInstallers(context.Background(), app(fromTenant, "addon"), as(controller))
	require.Len(t, errs, 1, "a tenant's nested Application cannot reach it through the controller")

	fromSystem := map[string]string{oam.LabelAppName: "addon-x", oam.LabelAppNamespace: oam.SystemDefinitionNamespace}
	require.Empty(t, h.ValidateAddonInstallers(context.Background(), app(fromSystem, "addon"), as(controller)),
		"an addon's own Application, in vela-system, may install the addons it depends on")
	require.Empty(t, h.ValidateAddonInstallers(context.Background(), app(nil, "addon"), as(controller)),
		"the controller writing for itself, outside dispatch")

	featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultMutableFeatureGate, features.EnableAddonComponent, false)
	require.Empty(t, h.ValidateAddonInstallers(context.Background(), app(nil, "addon"), as("tenant")), "the addon type does nothing with its gate off")
}

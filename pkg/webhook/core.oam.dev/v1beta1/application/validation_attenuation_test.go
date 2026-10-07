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
)

// askingClient records every access review the handler raises and allows them
// all, so a test can see exactly which definitions were asked about.
type askingClient struct {
	client.Client
	asked []string
}

func (c *askingClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	if sar, ok := obj.(*authv1.SubjectAccessReview); ok {
		c.asked = append(c.asked, sar.Spec.ResourceAttributes.Name)
		sar.Status.Allowed = true
		return nil
	}
	return c.Client.Create(ctx, obj, opts...)
}

// The permission check asks about what the Application names and nothing it
// extends.
//
// Granting a team an abstraction without the expressive definition under it is
// what inheritance is for: walking the chain would mean granting both, leaving
// nothing to attenuate. A parent lives in its child's own namespace, so writing
// a definition that extends a privileged one already takes authority over the
// namespace holding it.
func TestOnlyTheNamedDefinitionIsAskedAbout(t *testing.T) {
	featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate,
		features.ValidateDefinitionPermissions, true)

	scheme := runtime.NewScheme()
	_ = v1beta1.AddToScheme(scheme)
	_ = authv1.AddToScheme(scheme)

	// A real chain: acme-payments extends webservice, which exists.
	base := componentDef("vela-system", "webservice", "")
	paved := componentDef("vela-system", "acme-payments", "webservice")

	cli := &askingClient{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(base, paved).Build(),
	}
	h := &ValidatingHandler{Client: cli}

	app := &v1beta1.Application{
		ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "team-a"},
		Spec: v1beta1.ApplicationSpec{
			Components: []common.ApplicationComponent{{Name: "c", Type: "acme-payments"}},
		},
	}
	req := admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
		UserInfo: authenticationv1.UserInfo{Username: "someone"},
	}}

	errs := h.ValidateDefinitionPermissions(context.Background(), app, req)
	require.Empty(t, errs)

	require.Contains(t, cli.asked, "acme-payments")
	require.NotContains(t, cli.asked, "webservice",
		"asking about the parent would mean a team needed both to use one")
}

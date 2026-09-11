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
	"os"
	"testing"

	"github.com/kubevela/pkg/controller/sharding"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	admissionv1 "k8s.io/api/admission/v1"
	authenticationv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	featuregatetesting "k8s.io/component-base/featuregate/testing"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/features"
)

type fakeAddonComponentValidator struct {
	calls int
	errs  field.ErrorList
}

func (f *fakeAddonComponentValidator) ValidateComponents(
	_ context.Context,
	_ *v1beta1.Application,
) field.ErrorList {
	f.calls++
	return f.errs
}

func TestValidateAddonComponents(t *testing.T) {
	t.Run("skips validation when the feature gate is disabled", func(t *testing.T) {
		featuregatetesting.SetFeatureGateDuringTest(
			t, utilfeature.DefaultMutableFeatureGate, features.EnableAddonComponent, false,
		)
		validator := &fakeAddonComponentValidator{}
		handler := &ValidatingHandler{addonValidator: validator}
		app := addonApplication()

		errs := handler.ValidateAddonComponents(context.Background(), app)

		assert.Empty(t, errs)
		assert.Zero(t, validator.calls)
	})

	t.Run("skips validation when there are no addon components", func(t *testing.T) {
		featuregatetesting.SetFeatureGateDuringTest(
			t, utilfeature.DefaultMutableFeatureGate, features.EnableAddonComponent, true,
		)
		validator := &fakeAddonComponentValidator{}
		handler := &ValidatingHandler{addonValidator: validator}
		app := &v1beta1.Application{Spec: v1beta1.ApplicationSpec{
			Components: []common.ApplicationComponent{{Name: "web", Type: "webservice"}},
		}}

		errs := handler.ValidateAddonComponents(context.Background(), app)

		assert.Empty(t, errs)
		assert.Zero(t, validator.calls)
	})

	t.Run("delegates validation when an addon component is present", func(t *testing.T) {
		featuregatetesting.SetFeatureGateDuringTest(
			t, utilfeature.DefaultMutableFeatureGate, features.EnableAddonComponent, true,
		)
		expectedErrs := field.ErrorList{field.Invalid(field.NewPath("spec", "components").Index(0), "addon", "invalid addon")}
		validator := &fakeAddonComponentValidator{errs: expectedErrs}
		handler := &ValidatingHandler{addonValidator: validator}
		app := addonApplication()

		errs := handler.ValidateAddonComponents(context.Background(), app)

		assert.Equal(t, expectedErrs, errs)
		assert.Equal(t, 1, validator.calls)
	})
}

func TestValidateAddonComponentsAggregatesErrorsForCreateAndUpdate(t *testing.T) {
	featuregatetesting.SetFeatureGateDuringTest(
		t, utilfeature.DefaultMutableFeatureGate, features.EnableAddonComponent, true,
	)
	oldSharding := sharding.EnableSharding
	sharding.EnableSharding = true
	t.Cleanup(func() { sharding.EnableSharding = oldSharding })
	featuregatetesting.SetFeatureGateDuringTest(
		t, utilfeature.DefaultMutableFeatureGate, features.ValidateComponentWhenSharding, false,
	)

	addonErr := field.Invalid(field.NewPath("spec", "components").Index(0).Child("properties"), "addon", "invalid addon")
	validator := &fakeAddonComponentValidator{errs: field.ErrorList{addonErr}}
	scheme := runtime.NewScheme()
	_ = v1beta1.AddToScheme(scheme)
	handler := &ValidatingHandler{
		Client:         fake.NewClientBuilder().WithScheme(scheme).Build(),
		addonValidator: validator,
	}
	app := addonApplication()
	req := admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
		UserInfo: authenticationv1.UserInfo{Username: "test-user"},
	}}

	createErrs := handler.ValidateCreate(context.Background(), app, req)
	assert.Contains(t, createErrs, addonErr)
	assert.Equal(t, 1, validator.calls)

	updateErrs := handler.ValidateUpdate(context.Background(), app, app.DeepCopy(), req)
	assert.Contains(t, updateErrs, addonErr)
	assert.Equal(t, 2, validator.calls)
}

func TestAddonValidationUsesSharedApplicationWebhook(t *testing.T) {
	chart, err := os.ReadFile("../../../../../charts/vela-core/templates/admission-webhooks/validatingWebhookConfiguration.yaml")
	require.NoError(t, err)
	registry, err := os.ReadFile("../../register.go")
	require.NoError(t, err)

	assert.Contains(t, string(chart), "name: validating.core.oam.dev.v1beta1.applications")
	assert.NotContains(t, string(chart), "validating.core.oam.dev.v1beta1.addoncomponents")
	assert.NotContains(t, string(chart), "/validating-core-oam-dev-v1beta1-addon-components")
	assert.NotContains(t, string(registry), "addon.RegisterValidatingHandler")
}

func TestAddonValidatorLivesUnderApplication(t *testing.T) {
	_, err := os.Stat("addon/validator.go")
	require.NoError(t, err, "addon validator must live under the Application webhook package")

	_, err = os.Stat("../addon")
	require.ErrorIs(t, err, os.ErrNotExist,
		"top-level v1beta1/addon implies a standalone Addon webhook kind")
}

func addonApplication() *v1beta1.Application {
	return &v1beta1.Application{
		ObjectMeta: metav1.ObjectMeta{Name: "addon-app", Namespace: "default"},
		Spec: v1beta1.ApplicationSpec{
			Components: []common.ApplicationComponent{{Name: "addon", Type: "addon"}},
		},
	}
}

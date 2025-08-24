package definition

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	corev1beta1 "github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

func setupValidator(t *testing.T, objects ...runtime.Object) *DefinitionValidator {
	scheme := runtime.NewScheme()
	err := corev1beta1.AddToScheme(scheme)
	assert.NoError(t, err, "adding to scheme should not error")

	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build()
	validator := &DefinitionValidator{Client: client}
	validator.decoder, err = admission.NewDecoder(scheme)
	assert.NoError(t, err, "creating decoder should not error")

	return validator
}

func TestDefinitionValidator_ComponentDefinition(t *testing.T) {
	tests := []struct {
		name           string
		existingObject runtime.Object
		newVersion     string
		operation      admission.Operation
		wantAllowed    bool
	}{
		{
			name: "deny update for same version",
			existingObject: &corev1beta1.ComponentDefinition{
				ObjectMeta: metav1.ObjectMeta{Name: "test-component", Namespace: "vela-system"},
				Spec:       corev1beta1.ComponentDefinitionSpec{Version: "1.0.0"},
			},
			newVersion:  "1.0.0",
			operation:   admission.Update,
			wantAllowed: false,
		},
		{
			name: "allow update with new version",
			existingObject: &corev1beta1.ComponentDefinition{
				ObjectMeta: metav1.ObjectMeta{Name: "test-component", Namespace: "vela-system"},
				Spec:       corev1beta1.ComponentDefinitionSpec{Version: "1.0.0"},
			},
			newVersion:  "1.0.1",
			operation:   admission.Update,
			wantAllowed: true,
		},
		{
			name: "allow update for non-versioned definition",
			existingObject: &corev1beta1.ComponentDefinition{
				ObjectMeta: metav1.ObjectMeta{Name: "test-component", Namespace: "vela-system"},
				Spec:       corev1beta1.ComponentDefinitionSpec{Version: ""},
			},
			newVersion:  "",
			operation:   admission.Update,
			wantAllowed: true,
		},
		{
			name: "allow create operation",
			existingObject: &corev1beta1.ComponentDefinition{
				ObjectMeta: metav1.ObjectMeta{Name: "test-component", Namespace: "vela-system"},
				Spec:       corev1beta1.ComponentDefinitionSpec{Version: "1.0.0"},
			},
			newVersion:  "1.0.0",
			operation:   admission.Create,
			wantAllowed: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := setupValidator(t, tt.existingObject)

			req := admission.Request{
				Kind:      metav1.GroupVersionKind{Group: "core.oam.dev", Version: "v1beta1", Kind: "ComponentDefinition"},
				Namespace: "vela-system",
				Name:      "test-component",
				Operation: tt.operation,
				Object: runtime.RawExtension{
					Object: &corev1beta1.ComponentDefinition{
						ObjectMeta: metav1.ObjectMeta{Name: "test-component", Namespace: "vela-system"},
						Spec:       corev1beta1.ComponentDefinitionSpec{Version: tt.newVersion},
					},
				},
			}

			resp := validator.Handle(context.Background(), req)
			assert.Equal(t, tt.wantAllowed, resp.Allowed, "unexpected validation result")
		})
	}
}

func TestDefinitionValidator_TraitDefinition(t *testing.T) {
	tests := []struct {
		name           string
		existingObject runtime.Object
		newVersion     string
		operation      admission.Operation
		wantAllowed    bool
	}{
		{
			name: "deny update for same version",
			existingObject: &corev1beta1.TraitDefinition{
				ObjectMeta: metav1.ObjectMeta{Name: "test-trait", Namespace: "vela-system"},
				Spec:       corev1beta1.TraitDefinitionSpec{Version: "1.0.0"},
			},
			newVersion:  "1.0.0",
			operation:   admission.Update,
			wantAllowed: false,
		},
		{
			name: "allow update with new version",
			existingObject: &corev1beta1.TraitDefinition{
				ObjectMeta: metav1.ObjectMeta{Name: "test-trait", Namespace: "vela-system"},
				Spec:       corev1beta1.TraitDefinitionSpec{Version: "1.0.0"},
			},
			newVersion:  "1.0.1",
			operation:   admission.Update,
			wantAllowed: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := setupValidator(t, tt.existingObject)

			req := admission.Request{
				Kind:      metav1.GroupVersionKind{Group: "core.oam.dev", Version: "v1beta1", Kind: "TraitDefinition"},
				Namespace: "vela-system",
				Name:      "test-trait",
				Operation: tt.operation,
				Object: runtime.RawExtension{
					Object: &corev1beta1.TraitDefinition{
						ObjectMeta: metav1.ObjectMeta{Name: "test-trait", Namespace: "vela-system"},
						Spec:       corev1beta1.TraitDefinitionSpec{Version: tt.newVersion},
					},
				},
			}

			resp := validator.Handle(context.Background(), req)
			assert.Equal(t, tt.wantAllowed, resp.Allowed, "unexpected validation result")
		})
	}
}

func TestDefinitionValidator_PolicyDefinition(t *testing.T) {
	tests := []struct {
		name           string
		existingObject runtime.Object
		newVersion     string
		operation      admission.Operation
		wantAllowed    bool
	}{
		{
			name: "deny update for same version",
			existingObject: &corev1beta1.PolicyDefinition{
				ObjectMeta: metav1.ObjectMeta{Name: "test-policy", Namespace: "vela-system"},
				Spec:       corev1beta1.PolicyDefinitionSpec{Version: "1.0.0"},
			},
			newVersion:  "1.0.0",
			operation:   admission.Update,
			wantAllowed: false,
		},
		{
			name: "allow update with new version",
			existingObject: &corev1beta1.PolicyDefinition{
				ObjectMeta: metav1.ObjectMeta{Name: "test-policy", Namespace: "vela-system"},
				Spec:       corev1beta1.PolicyDefinitionSpec{Version: "1.0.0"},
			},
			newVersion:  "1.0.1",
			operation:   admission.Update,
			wantAllowed: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := setupValidator(t, tt.existingObject)

			req := admission.Request{
				Kind:      metav1.GroupVersionKind{Group: "core.oam.dev", Version: "v1beta1", Kind: "PolicyDefinition"},
				Namespace: "vela-system",
				Name:      "test-policy",
				Operation: tt.operation,
				Object: runtime.RawExtension{
					Object: &corev1beta1.PolicyDefinition{
						ObjectMeta: metav1.ObjectMeta{Name: "test-policy", Namespace: "vela-system"},
						Spec:       corev1beta1.PolicyDefinitionSpec{Version: tt.newVersion},
					},
				},
			}

			resp := validator.Handle(context.Background(), req)
			assert.Equal(t, tt.wantAllowed, resp.Allowed, "unexpected validation result")
		})
	}
}

func TestDefinitionValidator_UnsupportedKind(t *testing.T) {
	validator := setupValidator(t)

	req := admission.Request{
		Kind:      metav1.GroupVersionKind{Group: "core.oam.dev", Version: "v1beta1", Kind: "UnsupportedKind"},
		Namespace: "vela-system",
		Name:      "test-unsupported",
		Operation: admission.Update,
	}

	resp := validator.Handle(context.Background(), req)
	assert.True(t, resp.Allowed, "should allow unsupported kinds")
}

package definition

import (
	"context"
	"testing"

	corev1beta1 "github.com/kubevela/kubevela/apis/core.oam.dev/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

func TestDefinitionValidator_ComponentDefinition_ImmutableVersion(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1beta1.AddToScheme(scheme)
	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(&corev1beta1.ComponentDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "test-component", Namespace: "vela-system"},
		Spec:       corev1beta1.ComponentDefinitionSpec{Version: "1.0.0"},
	}).Build()

	validator := &DefinitionValidator{Client: client}
	validator.decoder, _ = admission.NewDecoder(scheme)

	req := admission.Request{
		Kind:      metav1.GroupVersionKind{Group: "core.oam.dev", Version: "v1beta1", Kind: "ComponentDefinition"},
		Namespace: "vela-system",
		Name:      "test-component",
		Operation: admission.Update,
		Object: runtime.RawExtension{
			Object: &corev1beta1.ComponentDefinition{
				ObjectMeta: metav1.ObjectMeta{Name: "test-component", Namespace: "vela-system"},
				Spec:       corev1beta1.ComponentDefinitionSpec{Version: "1.0.0"},
			},
		},
	}

	resp := validator.Handle(context.Background(), req)
	if resp.Allowed {
		t.Error("Expected update to be denied for same version")
	}
}

func TestDefinitionValidator_ComponentDefinition_AllowNewVersion(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1beta1.AddToScheme(scheme)
	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(&corev1beta1.ComponentDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "test-component", Namespace: "vela-system"},
		Spec:       corev1beta1.ComponentDefinitionSpec{Version: "1.0.0"},
	}).Build()

	validator := &DefinitionValidator{Client: client}
	validator.decoder, _ = admission.NewDecoder(scheme)

	req := admission.Request{
		Kind:      metav1.GroupVersionKind{Group: "core.oam.dev", Version: "v1beta1", Kind: "ComponentDefinition"},
		Namespace: "vela-system",
		Name:      "test-component",
		Operation: admission.Update,
		Object: runtime.RawExtension{
			Object: &corev1beta1.ComponentDefinition{
				ObjectMeta: metav1.ObjectMeta{Name: "test-component", Namespace: "vela-system"},
				Spec:       corev1beta1.ComponentDefinitionSpec{Version: "1.0.1"},
			},
		},
	}

	resp := validator.Handle(context.Background(), req)
	if !resp.Allowed {
		t.Error("Expected update to be allowed for new version")
	}
}

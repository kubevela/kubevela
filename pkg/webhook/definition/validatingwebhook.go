package definition

import (
	"context"
	"fmt"
	"net/http"

	corev1beta1 "github.com/kubevela/kubevela/apis/core.oam.dev/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// DefinitionValidator validates immutability of definition versions.
type DefinitionValidator struct {
	Client  client.Client
	decoder *admission.Decoder
}

func (v *DefinitionValidator) Handle(ctx context.Context, req admission.Request) admission.Response {
	var obj runtime.Object
	switch req.Kind.Kind {
	case "ComponentDefinition":
		obj = &corev1beta1.ComponentDefinition{}
	case "TraitDefinition":
		obj = &corev1beta1.TraitDefinition{}
	case "PolicyDefinition":
		obj = &corev1beta1.PolicyDefinition{}
	default:
		return admission.Allowed("Not a supported definition kind")
	}
	if err := v.decoder.Decode(req, obj); err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	// Only check on update
	if req.Operation != admission.Update {
		return admission.Allowed("Not an update operation")
	}

	// Get the old object from cluster
	oldObj := obj.DeepCopyObject()
	if err := v.Client.Get(ctx, client.ObjectKey{Namespace: req.Namespace, Name: req.Name}, oldObj); err != nil {
		return admission.Allowed("Definition does not exist, allow")
	}

	var oldVersion, newVersion string
	switch o := oldObj.(type) {
	case *corev1beta1.ComponentDefinition:
		oldVersion = o.Spec.Version
		newVersion = obj.(*corev1beta1.ComponentDefinition).Spec.Version
	case *corev1beta1.TraitDefinition:
		oldVersion = o.Spec.Version
		newVersion = obj.(*corev1beta1.TraitDefinition).Spec.Version
	case *corev1beta1.PolicyDefinition:
		oldVersion = o.Spec.Version
		newVersion = obj.(*corev1beta1.PolicyDefinition).Spec.Version
	}

	if oldVersion != "" && oldVersion == newVersion {
		return admission.Denied(fmt.Sprintf("Definition with version %s is immutable and cannot be updated", oldVersion))
	}
	return admission.Allowed("Definition update allowed")
}

func (v *DefinitionValidator) InjectDecoder(d *admission.Decoder) error {
	v.decoder = d
	return nil
}

func SetupWebhook(mgr ctrl.Manager) error {
	// Register for ComponentDefinition, TraitDefinition, PolicyDefinition
	hook := &DefinitionValidator{Client: mgr.GetClient()}
	return ctrl.NewWebhookManagedBy(mgr).
		For(&corev1beta1.ComponentDefinition{}).
		For(&corev1beta1.TraitDefinition{}).
		For(&corev1beta1.PolicyDefinition{}).
		WithValidator(hook).
		Complete()
}

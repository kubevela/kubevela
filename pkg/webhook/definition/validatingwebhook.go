package definition

import (
	"context"
	"fmt"
	"net/http"

	corev1beta1 "github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// DefinitionValidator validates immutability of definition versions.
type DefinitionValidator struct {
	Client  client.Client
	decoder *admission.Decoder
}

// Handle validates that definition versions are immutable
func (v *DefinitionValidator) Handle(ctx context.Context, req admission.Request) admission.Response {
	logger := log.FromContext(ctx).WithValues("webhook", "definition-validator", "name", req.Name, "namespace", req.Namespace)

	var obj runtime.Object
	switch req.Kind.Kind {
	case "ComponentDefinition":
		obj = &corev1beta1.ComponentDefinition{}
	case "TraitDefinition":
		obj = &corev1beta1.TraitDefinition{}
	case "PolicyDefinition":
		obj = &corev1beta1.PolicyDefinition{}
	default:
		logger.Info("Not a supported definition kind", "kind", req.Kind.Kind)
		return admission.Allowed("Not a supported definition kind")
	}

	// Only check on update
	if req.Operation != admission.Update {
		logger.V(4).Info("Not an update operation", "operation", req.Operation)
		return admission.Allowed("Not an update operation")
	}

	if err := v.decoder.Decode(req, obj); err != nil {
		logger.Error(err, "Failed to decode request")
		return admission.Errored(http.StatusBadRequest, err)
	}

	// Get the old object from cluster
	oldObj := obj.DeepCopyObject()
	if err := v.Client.Get(ctx, client.ObjectKey{Namespace: req.Namespace, Name: req.Name}, oldObj); err != nil {
		logger.Info("Definition does not exist, allowing update", "error", err)
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

	// If both versions are set and they're the same, deny the update
	if oldVersion != "" && newVersion != "" && oldVersion == newVersion {
		logger.Info("Denying update of definition with immutable version",
			"kind", req.Kind.Kind,
			"version", oldVersion)
		return admission.Denied(fmt.Sprintf("Definition with version %s is immutable and cannot be updated", oldVersion))
	}

	logger.V(4).Info("Allowing definition update",
		"kind", req.Kind.Kind,
		"oldVersion", oldVersion,
		"newVersion", newVersion)
	return admission.Allowed("Definition update allowed")
}

// InjectDecoder injects the decoder into the validator
func (v *DefinitionValidator) InjectDecoder(d *admission.Decoder) error {
	v.decoder = d
	return nil
}

// SetupWebhook registers the validating webhook for all definition types
func SetupWebhook(mgr ctrl.Manager) error {
	logger := mgr.GetLogger().WithName("webhook").WithName("definition-validator")
	logger.Info("Setting up definition validator webhook")

	// Register for ComponentDefinition, TraitDefinition, PolicyDefinition
	hook := &DefinitionValidator{Client: mgr.GetClient()}
	return ctrl.NewWebhookManagedBy(mgr).
		For(&corev1beta1.ComponentDefinition{}).
		For(&corev1beta1.TraitDefinition{}).
		For(&corev1beta1.PolicyDefinition{}).
		WithValidator(hook).
		Complete()
}

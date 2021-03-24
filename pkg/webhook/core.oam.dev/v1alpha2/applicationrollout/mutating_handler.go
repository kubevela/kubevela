package applicationrollout

import (
	"context"
	"encoding/json"
	"net/http"

	"k8s.io/klog/v2"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/runtime/inject"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/pkg/controller/common"
	util "github.com/oam-dev/kubevela/pkg/utils"
	"github.com/oam-dev/kubevela/pkg/webhook/common/rollout"
)

// MutatingHandler handles AppRollout
type MutatingHandler struct {
	Client client.Client

	// Decoder decodes objects
	Decoder *admission.Decoder
}

var _ admission.Handler = &MutatingHandler{}

// Handle handles admission requests.
func (h *MutatingHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
	obj := &v1alpha2.AppRollout{}

	err := h.Decoder.Decode(req, obj)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}
	DefaultAppRollout(obj)

	marshalled, err := json.Marshal(obj)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}
	resp := admission.PatchResponseFromRaw(req.AdmissionRequest.Object.Raw, marshalled)
	if len(resp.Patches) > 0 {
		klog.V(common.LogDebugWithContent).Infof("Admit AppRollout %s/%s patches: %v", obj.Namespace, obj.Name,
			util.DumpJSON(resp.Patches))
	}
	return resp
}

// DefaultAppRollout will set the default value for the AppRollout®
func DefaultAppRollout(obj *v1alpha2.AppRollout) {
	klog.InfoS("default", "name", obj.Name)
	if obj.Spec.RevertOnDelete == nil {
		klog.V(common.LogDebug).Info("default RevertOnDelete as false")
		obj.Spec.RevertOnDelete = pointer.BoolPtr(false)
	}

	// default rollout plan
	rollout.DefaultRolloutPlan(&obj.Spec.RolloutPlan)
}

var _ inject.Client = &MutatingHandler{}

// InjectClient injects the client into the MutatingHandler
func (h *MutatingHandler) InjectClient(c client.Client) error {
	h.Client = c
	return nil
}

var _ admission.DecoderInjector = &MutatingHandler{}

// InjectDecoder injects the decoder into the MutatingHandler
func (h *MutatingHandler) InjectDecoder(d *admission.Decoder) error {
	h.Decoder = d
	return nil
}

// RegisterMutatingHandler will register component mutation handler to the webhook
func RegisterMutatingHandler(mgr manager.Manager) {
	server := mgr.GetWebhookServer()
	server.Register("/mutating-core-oam-dev-v1alpha2-approllout",
		&webhook.Admission{Handler: &MutatingHandler{}})
}

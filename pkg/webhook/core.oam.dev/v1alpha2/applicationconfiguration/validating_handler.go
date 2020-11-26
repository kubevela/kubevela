package applicationconfiguration

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
	"github.com/crossplane/oam-kubernetes-runtime/pkg/oam"
	"github.com/crossplane/oam-kubernetes-runtime/pkg/oam/discoverymapper"

	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/runtime/inject"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

const (
	errFmtWorkloadNameNotEmpty = "versioning-enabled component's workload name MUST NOT be assigned, expect workload name %q to be empty."

	errFmtRevisionName = "componentName %q and revisionName %q are mutually exclusive, you can only specify one of them"

	errFmtUnappliableTrait = "the trait %q cannot apply to workload %q of component %q (appliable: %q)"

	// WorkloadNamePath indicates field path of workload name
	WorkloadNamePath = "metadata.name"
)

var appConfigResource = v1alpha2.SchemeGroupVersion.WithResource("applicationconfigurations")

// AppConfigValidator provides functions to validate ApplicationConfiguration
type AppConfigValidator interface {
	Validate(context.Context, ValidatingAppConfig) []error
}

// AppConfigValidateFunc implements function to validate ApplicationConfiguration
type AppConfigValidateFunc func(context.Context, ValidatingAppConfig) []error

// Validate validates ApplicationConfiguration
func (fn AppConfigValidateFunc) Validate(ctx context.Context, v ValidatingAppConfig) []error {
	return fn(ctx, v)
}

// ValidatingHandler handles CloneSet
type ValidatingHandler struct {
	Client client.Client
	Mapper discoverymapper.DiscoveryMapper

	// Decoder decodes objects
	Decoder *admission.Decoder

	Validators []AppConfigValidator
}

var _ admission.Handler = &ValidatingHandler{}

// Handle validate ApplicationConfiguration Spec here
func (h *ValidatingHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
	obj := &v1alpha2.ApplicationConfiguration{}
	if req.Resource.String() != appConfigResource.String() {
		return admission.Errored(http.StatusBadRequest, fmt.Errorf("expect resource to be %s", appConfigResource))
	}
	switch req.Operation { //nolint:exhaustive
	case admissionv1beta1.Delete:
		if len(req.OldObject.Raw) != 0 {
			if err := h.Decoder.DecodeRaw(req.OldObject, obj); err != nil {
				return admission.Errored(http.StatusBadRequest, err)
			}
		} else {
			// TODO(wonderflow): we can audit delete or something else here.
			klog.Info("deleting Application Configuration", req.Name)
		}
	default:
		err := h.Decoder.Decode(req, obj)
		if err != nil {
			return admission.Errored(http.StatusBadRequest, err)
		}
		vAppConfig := &ValidatingAppConfig{}
		if err := vAppConfig.PrepareForValidation(ctx, h.Client, h.Mapper, obj); err != nil {
			klog.Info("failed init appConfig before validation ", " name: ", obj.Name, " errMsg: ", err.Error())
			return admission.Denied(err.Error())
		}
		for _, validator := range h.Validators {
			if allErrs := validator.Validate(ctx, *vAppConfig); utilerrors.NewAggregate(allErrs) != nil {
				// utilerrors.NewAggregate can remove nil from allErrs
				klog.Info("validation failed ", " name: ", obj.Name, " errMsgi: ", utilerrors.NewAggregate(allErrs).Error())
				return admission.Denied(utilerrors.NewAggregate(allErrs).Error())
			}
		}
	}
	return admission.ValidationResponse(true, "")
}

// ValidateTraitObjectFn validates the ApplicationConfiguration on creation/update
func ValidateTraitObjectFn(_ context.Context, v ValidatingAppConfig) []error {
	klog.Info("validate applicationConfiguration", "name", v.appConfig.Name)
	var allErrs field.ErrorList
	for cidx, comp := range v.validatingComps {
		for idx, tr := range comp.validatingTraits {
			fldPath := field.NewPath("spec").Child("components").Index(cidx).Child("traits").Index(idx).Child("trait")
			content := tr.traitContent.Object
			if content[TraitTypeField] != nil {
				allErrs = append(allErrs, field.Invalid(fldPath, string(tr.componentTrait.Trait.Raw),
					"the trait contains 'name' info that should be mutated to GVK"))
			}
			if content[TraitSpecField] != nil {
				allErrs = append(allErrs, field.Invalid(fldPath, string(tr.componentTrait.Trait.Raw),
					"the trait contains 'properties' info that should be mutated to spec"))
			}
			if len(tr.traitContent.GetAPIVersion()) == 0 || len(tr.traitContent.GetKind()) == 0 {
				allErrs = append(allErrs, field.Invalid(fldPath, content,
					fmt.Sprintf("the trait data missing GVK, api = %s, kind = %s,",
						tr.traitContent.GetAPIVersion(), tr.traitContent.GetKind())))
			}
		}
	}
	if len(allErrs) > 0 {
		return allErrs.ToAggregate().Errors()
	}
	return nil
}

// ValidateRevisionNameFn validates revisionName and componentName are assigned both.
func ValidateRevisionNameFn(_ context.Context, v ValidatingAppConfig) []error {
	klog.Info("validate revisionName in applicationConfiguration", "name", v.appConfig.Name)
	var allErrs []error
	for _, c := range v.validatingComps {
		if c.appConfigComponent.ComponentName != "" && c.appConfigComponent.RevisionName != "" {
			allErrs = append(allErrs, fmt.Errorf(errFmtRevisionName,
				c.appConfigComponent.ComponentName, c.appConfigComponent.RevisionName))
		}
	}
	return allErrs
}

// ValidateWorkloadNameForVersioningFn validates workload name for version-enabled component
func ValidateWorkloadNameForVersioningFn(_ context.Context, v ValidatingAppConfig) []error {
	var allErrs []error
	for _, c := range v.validatingComps {
		isVersionEnabled := false
		for _, t := range c.validatingTraits {
			if t.traitDefinition.Spec.RevisionEnabled {
				isVersionEnabled = true
				break
			}
		}
		if isVersionEnabled {
			if ok, workloadName := checkParams(c.component.Spec.Parameters, c.appConfigComponent.ParameterValues); !ok {
				allErrs = append(allErrs, fmt.Errorf(errFmtWorkloadNameNotEmpty, workloadName))
			}
			if workloadName := c.workloadContent.GetName(); workloadName != "" {
				allErrs = append(allErrs, fmt.Errorf(errFmtWorkloadNameNotEmpty, workloadName))
			}
		}
	}
	return allErrs
}

// ValidateTraitAppliableToWorkloadFn validates whether a trait is allowed to apply to the workload.
func ValidateTraitAppliableToWorkloadFn(_ context.Context, v ValidatingAppConfig) []error {
	klog.Info("validate trait is appliable to workload", "name", v.appConfig.Name)
	var allErrs []error
	for _, c := range v.validatingComps {
		workloadType := c.component.GetLabels()[oam.WorkloadTypeLabel]
		workloadDefRefName := c.workloadDefinition.Spec.Reference.Name
		// TODO(roywang) consider a CRD group could have multiple versions
		// and maybe we need to specify the minimum version here in the future
		workloadGroup := c.workloadDefinition.GetObjectKind().GroupVersionKind().Group
	ValidateApplyTo:
		for _, t := range c.validatingTraits {
			if len(t.traitDefinition.Spec.AppliesToWorkloads) == 0 {
				// AppliesToWorkloads is empty, the trait can be applied to ANY workload
				continue
			}
			for _, applyTo := range t.traitDefinition.Spec.AppliesToWorkloads {
				if applyTo == "*" {
					// "*" means the trait can be applied to ANY workload
					continue ValidateApplyTo
				}
				if strings.HasPrefix(applyTo, "*.") && workloadGroup == applyTo[2:] {
					continue ValidateApplyTo
				}
				if workloadType == applyTo ||
					workloadDefRefName == applyTo {
					continue ValidateApplyTo
				}
			}
			allErrs = append(allErrs, fmt.Errorf(errFmtUnappliableTrait,
				t.traitDefinition.GetObjectKind().GroupVersionKind().String(),
				c.workloadDefinition.GetObjectKind().GroupVersionKind().String(),
				c.compName, t.traitDefinition.Spec.AppliesToWorkloads))
		}
	}
	return allErrs
}

var _ inject.Client = &ValidatingHandler{}

// InjectClient injects the client into the ValidatingHandler
func (h *ValidatingHandler) InjectClient(c client.Client) error {
	h.Client = c
	return nil
}

var _ admission.DecoderInjector = &ValidatingHandler{}

// InjectDecoder injects the decoder into the ValidatingHandler
func (h *ValidatingHandler) InjectDecoder(d *admission.Decoder) error {
	h.Decoder = d
	return nil
}

// RegisterValidatingHandler will register application configuration validation to webhook
func RegisterValidatingHandler(mgr manager.Manager) error {
	server := mgr.GetWebhookServer()
	mapper, err := discoverymapper.New(mgr.GetConfig())
	if err != nil {
		return err
	}
	server.Register("/validating-core-oam-dev-v1alpha2-applicationconfigurations", &webhook.Admission{Handler: &ValidatingHandler{
		Mapper: mapper,
		Validators: []AppConfigValidator{
			AppConfigValidateFunc(ValidateTraitObjectFn),
			AppConfigValidateFunc(ValidateRevisionNameFn),
			AppConfigValidateFunc(ValidateWorkloadNameForVersioningFn),
			AppConfigValidateFunc(ValidateTraitAppliableToWorkloadFn),
			// TODO(wonderflow): Add more validation logic here.
		},
	}})
	return nil
}

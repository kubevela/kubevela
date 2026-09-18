/*
Copyright 2021 The KubeVela Authors.

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

package docgen

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"

	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/format"
	"cuelang.org/go/cue/parser"
	"github.com/pkg/errors"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	commontypes "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/appfile"
	"github.com/oam-dev/kubevela/pkg/cue"
	"github.com/oam-dev/kubevela/pkg/definition"
	"github.com/oam-dev/kubevela/pkg/definition/cachekey"
	"github.com/oam-dev/kubevela/pkg/definition/propexpr"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/pkg/sources"
	"github.com/oam-dev/kubevela/pkg/utils"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	"github.com/oam-dev/kubevela/pkg/webhook/core.oam.dev/v1beta1/sourcedefinition"
	"github.com/oam-dev/kubevela/references/docgen/fix"
)

// DescriptionUndefined indicates the description is not defined
const DescriptionUndefined = "description not defined"

// GetCapabilitiesFromCluster will get capability from K8s cluster
func GetCapabilitiesFromCluster(ctx context.Context, namespace string, c common.Args, selector labels.Selector) ([]types.Capability, error) {
	caps, erl, err := GetComponentsFromCluster(ctx, namespace, c, selector)
	for _, er := range erl {
		klog.Infof("get component capability %v", er)
	}
	if err != nil {
		return nil, err
	}

	traits, erl, err := GetTraitsFromCluster(ctx, namespace, c, selector)
	if err != nil {
		return nil, err
	}
	for _, er := range erl {
		klog.Infof("get trait capability %v", er)
	}
	caps = append(caps, traits...)

	plcs, erl, err := GetPolicies(ctx, namespace, c)
	if err != nil {
		return nil, err
	}
	for _, er := range erl {
		klog.Infof("get policy capability %v", er)
	}
	caps = append(caps, plcs...)

	wfs, erl, err := GetWorkflowSteps(ctx, namespace, c)
	if err != nil {
		return nil, err
	}
	for _, er := range erl {
		klog.Infof("get workflow step %v", er)
	}
	caps = append(caps, wfs...)

	srcs, erl, err := GetSources(ctx, namespace, c)
	if err != nil {
		return nil, err
	}
	for _, er := range erl {
		klog.Infof("get source capability %v", er)
	}
	caps = append(caps, srcs...)

	return caps, nil
}

// GetNamespacedCapabilitiesFromCluster will get capability from K8s cluster in the specified namespace and default namespace
// If the definition could be found from `namespace`, try to find in namespace `types.DefaultKubeVelaNS`
func GetNamespacedCapabilitiesFromCluster(ctx context.Context, namespace string, c common.Args, selector labels.Selector) ([]types.Capability, error) {
	var capabilities []types.Capability

	if workloads, _, err := GetComponentsFromClusterWithValidateOption(ctx, namespace, c, selector, false); err == nil {
		capabilities = append(capabilities, workloads...)
	}

	if traits, _, err := GetTraitsFromClusterWithValidateOption(ctx, namespace, c, selector, false); err == nil {
		capabilities = append(capabilities, traits...)
	}

	if workflowSteps, _, err := GetWorkflowSteps(ctx, namespace, c); err == nil {
		capabilities = append(capabilities, workflowSteps...)
	}

	if policies, _, err := GetPolicies(ctx, namespace, c); err == nil {
		capabilities = append(capabilities, policies...)
	}

	if sources, _, err := GetSources(ctx, namespace, c); err == nil {
		capabilities = append(capabilities, sources...)
	}

	if namespace != types.DefaultKubeVelaNS {
		// get components from default namespace
		if workloads, _, err := GetComponentsFromClusterWithValidateOption(ctx, types.DefaultKubeVelaNS, c, selector, false); err == nil {
			capabilities = append(capabilities, workloads...)
		}

		// get traits from default namespace
		if traits, _, err := GetTraitsFromClusterWithValidateOption(ctx, types.DefaultKubeVelaNS, c, selector, false); err == nil {
			capabilities = append(capabilities, traits...)
		}

		if workflowSteps, _, err := GetWorkflowSteps(ctx, types.DefaultKubeVelaNS, c); err == nil {
			capabilities = append(capabilities, workflowSteps...)
		}

		if policies, _, err := GetPolicies(ctx, types.DefaultKubeVelaNS, c); err == nil {
			capabilities = append(capabilities, policies...)
		}

		if sources, _, err := GetSources(ctx, types.DefaultKubeVelaNS, c); err == nil {
			capabilities = append(capabilities, sources...)
		}
	}

	if len(capabilities) > 0 {
		return capabilities, nil
	}
	return nil, fmt.Errorf("could not find any components, traits, workflowSteps, policies or sources from namespace %s and %s", namespace, types.DefaultKubeVelaNS)
}

// GetComponentsFromCluster will get capability from K8s cluster
func GetComponentsFromCluster(ctx context.Context, namespace string, c common.Args, selector labels.Selector) ([]types.Capability, []error, error) {
	return GetComponentsFromClusterWithValidateOption(ctx, namespace, c, selector, true)
}

// GetComponentsFromClusterWithValidateOption will get capability from K8s cluster with an option whether to valid Components
func GetComponentsFromClusterWithValidateOption(ctx context.Context, namespace string, c common.Args, selector labels.Selector, validateFlag bool) ([]types.Capability, []error, error) {
	newClient, err := c.GetClient()
	if err != nil {
		return nil, nil, err
	}

	var templates []types.Capability
	var componentsDefs v1beta1.ComponentDefinitionList
	err = newClient.List(ctx, &componentsDefs, &client.ListOptions{Namespace: namespace, LabelSelector: selector})
	if err != nil {
		return nil, nil, fmt.Errorf("list ComponentDefinition err: %w", err)
	}

	var templateErrors []error
	for _, cd := range componentsDefs.Items {
		defRef := commontypes.DefinitionReference{
			Name: cd.Spec.Workload.Type,
		}
		if cd.Spec.Workload.Type != types.AutoDetectWorkloadDefinition {
			defRef, err = util.ConvertWorkloadGVK2Definition(newClient.RESTMapper(), cd.Spec.Workload.Definition)
			if err != nil {
				return nil, nil, err
			}
		}

		tmp, err := GetCapabilityByComponentDefinitionObject(cd, defRef.Name)
		if err != nil {
			templateErrors = append(templateErrors, err)
			continue
		}
		if validateFlag && defRef.Name != types.AutoDetectWorkloadDefinition {
			if err = validateCapabilities(newClient.RESTMapper(), cd.Name, defRef); err != nil {
				return nil, nil, err
			}
		}
		templates = append(templates, *tmp)
	}
	return templates, templateErrors, nil
}

// GetTraitsFromCluster will get capability from K8s cluster
func GetTraitsFromCluster(ctx context.Context, namespace string, c common.Args, selector labels.Selector) ([]types.Capability, []error, error) {
	return GetTraitsFromClusterWithValidateOption(ctx, namespace, c, selector, true)
}

// GetTraitsFromClusterWithValidateOption will get capability from K8s cluster with an option whether to valid Traits
func GetTraitsFromClusterWithValidateOption(ctx context.Context, namespace string, c common.Args, selector labels.Selector, validateFlag bool) ([]types.Capability, []error, error) {
	newClient, err := c.GetClient()
	if err != nil {
		return nil, nil, err
	}
	var templates []types.Capability
	var traitDefs v1beta1.TraitDefinitionList
	err = newClient.List(ctx, &traitDefs, &client.ListOptions{Namespace: namespace, LabelSelector: selector})
	if err != nil {
		return nil, nil, fmt.Errorf("list TraitDefinition err: %w", err)
	}

	var templateErrors []error
	for _, td := range traitDefs.Items {
		var tmp *types.Capability
		var err error
		// FIXME: remove this temporary fix when https://github.com/cue-lang/cue/issues/2047 is fixed
		if td.Name == "container-image" {
			tmp = fix.CapContainerImage
		} else {
			tmp, err = GetCapabilityByTraitDefinitionObject(td)
			if err != nil {
				templateErrors = append(templateErrors, errors.Wrapf(err, "handle trait template `%s` failed", td.Name))
				continue
			}
		}
		tmp.Namespace = namespace
		if validateFlag {
			if err = validateCapabilities(newClient.RESTMapper(), td.Name, td.Spec.Reference); err != nil {
				return nil, nil, err
			}
		}
		templates = append(templates, *tmp)
	}
	return templates, templateErrors, nil
}

// GetWorkflowSteps will get WorkflowStepDefinition list
func GetWorkflowSteps(ctx context.Context, namespace string, c common.Args) ([]types.Capability, []error, error) {
	newClient, err := c.GetClient()
	if err != nil {
		return nil, nil, err
	}

	var templates []types.Capability
	var workflowStepDefs v1beta1.WorkflowStepDefinitionList
	err = newClient.List(ctx, &workflowStepDefs, &client.ListOptions{Namespace: namespace})
	if err != nil {
		return nil, nil, fmt.Errorf("list WorkflowStepDefinition err: %w", err)
	}

	var templateErrors []error
	for _, def := range workflowStepDefs.Items {
		tmp, err := GetCapabilityByWorkflowStepDefinitionObject(def)
		if err != nil {
			templateErrors = append(templateErrors, errors.WithMessage(err, def.Name))
			continue
		}
		templates = append(templates, *tmp)
	}
	return templates, templateErrors, nil
}

// GetPolicies will get Policy from K8s cluster
func GetPolicies(ctx context.Context, namespace string, c common.Args) ([]types.Capability, []error, error) {
	newClient, err := c.GetClient()
	if err != nil {
		return nil, nil, err
	}

	var templates []types.Capability
	var defs v1beta1.PolicyDefinitionList
	err = newClient.List(ctx, &defs, &client.ListOptions{Namespace: namespace})
	if err != nil {
		return nil, nil, fmt.Errorf("list PolicyDefinition err: %w", err)
	}

	var templateErrors []error
	for _, def := range defs.Items {
		tmp, err := GetCapabilityByPolicyDefinitionObject(def)
		if err != nil {
			templateErrors = append(templateErrors, err)
			continue
		}
		templates = append(templates, *tmp)
	}
	return templates, templateErrors, nil
}

// GetSources gets SourceDefinitions from cluster
func GetSources(ctx context.Context, namespace string, c common.Args) ([]types.Capability, []error, error) {
	newClient, err := c.GetClient()
	if err != nil {
		return nil, nil, err
	}

	var templates []types.Capability
	var defs v1beta1.SourceDefinitionList
	err = newClient.List(ctx, &defs, &client.ListOptions{Namespace: namespace})
	if err != nil {
		return nil, nil, fmt.Errorf("list SourceDefinition err: %w", err)
	}

	var templateErrors []error
	for _, def := range defs.Items {
		tmp, err := GetCapabilityBySourceDefinitionObject(def)
		if err != nil {
			templateErrors = append(templateErrors, err)
			continue
		}
		templates = append(templates, *tmp)
	}
	return templates, templateErrors, nil
}

// validateCapabilities validates whether GVK are successfully retrieved.
func validateCapabilities(mapper meta.RESTMapper, definitionName string, reference commontypes.DefinitionReference) error {
	_, err := util.GetGVKFromDefinition(mapper, reference)
	if err != nil {
		errMsg := err.Error()
		var substr = "no matches for "
		if strings.Contains(errMsg, substr) {
			return fmt.Errorf("expected provider: %s", strings.Split(errMsg, substr)[1])
		}
		return fmt.Errorf("installing capability '%s'... %w", definitionName, err)
	}
	return nil
}

// HandleDefinition will handle definition to capability
func HandleDefinition(name, crdName string, annotation, labels map[string]string, extension *runtime.RawExtension, tp types.CapType,
	applyTo []string, schematic *commontypes.Schematic) (types.Capability, error) {
	var tmp types.Capability
	tmp, err := HandleTemplate(extension, schematic, name)
	if err != nil {
		return types.Capability{}, err
	}
	tmp.Type = tp
	if tp == types.TypeTrait {
		tmp.AppliesTo = applyTo
	}
	tmp.CrdName = crdName
	tmp.Description = GetDescription(annotation)
	tmp.Example = GetExample(annotation)
	tmp.Labels = labels
	return tmp, nil
}

// GetDescription get description from annotation
func GetDescription(annotation map[string]string) string {
	if annotation == nil {
		return DescriptionUndefined
	}
	desc, ok := annotation[types.AnnoDefinitionDescription]
	if !ok {
		return DescriptionUndefined
	}
	desc = strings.ReplaceAll(desc, "\n", " ")
	return desc
}

// GetExample get example markdown from annotation specified url
func GetExample(annotation map[string]string) string {
	if annotation == nil {
		return ""
	}
	examplePath, ok := annotation[types.AnnoDefinitionExampleURL]
	if !ok {
		return ""
	}
	if !utils.IsValidURL(examplePath) {
		return ""
	}
	data, err := common.HTTPGetWithOption(context.Background(), examplePath, nil)
	if err != nil {
		return ""
	}
	if strings.HasSuffix(examplePath, ".yaml") {
		return fmt.Sprintf("```yaml\n%s\n```", string(data))
	}
	return string(data)
}

// HandleTemplate will handle definition template to capability
func HandleTemplate(in *runtime.RawExtension, schematic *commontypes.Schematic, name string) (types.Capability, error) {
	tmp, err := appfile.ConvertTemplateJSON2Object(name, in, schematic)
	if err != nil {
		return types.Capability{}, err
	}
	tmp.Name = name
	// if spec.template is not empty it should has the highest priority
	if schematic != nil {
		if schematic.CUE != nil {
			tmp.CueTemplate = schematic.CUE.Template
			tmp.CueTemplateURI = ""
		}
		if schematic.Terraform != nil {
			tmp.Category = types.TerraformCategory
			tmp.TerraformConfiguration = schematic.Terraform.Configuration
			tmp.ConfigurationType = schematic.Terraform.Type
			tmp.Path = schematic.Terraform.Path
			return tmp, nil
		}
	}
	if tmp.CueTemplateURI != "" {
		b, err := common.HTTPGetWithOption(context.Background(), tmp.CueTemplateURI, nil)
		if err != nil {
			return types.Capability{}, err
		}
		tmp.CueTemplate = string(b)
	}
	if tmp.CueTemplate == "" {
		return types.Capability{}, errors.New("template not exist in definition")
	}
	// TODO: Accept context parameter for proper cancellation/timeout support
	// Currently using Background() to avoid breaking changes to function
	tmp.Parameters, err = cue.GetParametersWithCuex(context.Background(), tmp.CueTemplate)
	if err != nil && !errors.Is(err, cue.ErrParameterNotExist) {
		return types.Capability{}, err
	}
	tmp.Category = types.CUECategory
	return tmp, nil
}

// GetCapabilityByName gets capability by definition name
func GetCapabilityByName(ctx context.Context, c common.Args, capabilityName string, ns string) (*types.Capability, error) {
	var (
		foundCapability bool
		capability      *types.Capability
		err             error
	)

	newClient, err := c.GetClient()
	if err != nil {
		return nil, err
	}
	var componentDef v1beta1.ComponentDefinition
	err = newClient.Get(ctx, client.ObjectKey{Namespace: ns, Name: capabilityName}, &componentDef)
	if err == nil {
		foundCapability = true
	} else if kerrors.IsNotFound(err) {
		err = newClient.Get(ctx, client.ObjectKey{Namespace: types.DefaultKubeVelaNS, Name: capabilityName}, &componentDef)
		if err == nil {
			foundCapability = true
		}
	}

	if foundCapability {
		var refName string

		// if workload type of ComponentDefinition is unclear,
		// set the DefinitionReference's Name to AutoDetectWorkloadDefinition
		if componentDef.Spec.Workload.Type == types.AutoDetectWorkloadDefinition {
			refName = types.AutoDetectWorkloadDefinition
		} else {
			ref, err := util.ConvertWorkloadGVK2Definition(newClient.RESTMapper(), componentDef.Spec.Workload.Definition)
			if err != nil {
				return nil, err
			}
			refName = ref.Name
		}

		capability, err = GetCapabilityByComponentDefinitionObject(componentDef, refName)
		if err != nil {
			return nil, err
		}
		return capability, nil
	}

	foundCapability = false
	var traitDef v1beta1.TraitDefinition
	err = newClient.Get(ctx, client.ObjectKey{Namespace: ns, Name: capabilityName}, &traitDef)
	if err == nil {
		foundCapability = true
	} else if kerrors.IsNotFound(err) {
		err = newClient.Get(ctx, client.ObjectKey{Namespace: types.DefaultKubeVelaNS, Name: capabilityName}, &traitDef)
		if err == nil {
			foundCapability = true
		}
	}
	if foundCapability {
		capability, err = GetCapabilityByTraitDefinitionObject(traitDef)
		if err != nil {
			return nil, err
		}
		return capability, nil
	}

	var wfStepDef v1beta1.WorkflowStepDefinition
	err = newClient.Get(ctx, client.ObjectKey{Namespace: ns, Name: capabilityName}, &wfStepDef)
	if err == nil {
		foundCapability = true
	} else if kerrors.IsNotFound(err) {
		err = newClient.Get(ctx, client.ObjectKey{Namespace: types.DefaultKubeVelaNS, Name: capabilityName}, &wfStepDef)
		if err == nil {
			foundCapability = true
		}
	}
	if foundCapability {
		capability, err = GetCapabilityByWorkflowStepDefinitionObject(wfStepDef)
		if err != nil {
			return nil, err
		}
		return capability, nil
	}

	var policyDef v1beta1.PolicyDefinition
	err = newClient.Get(ctx, client.ObjectKey{Namespace: ns, Name: capabilityName}, &policyDef)
	if err == nil {
		foundCapability = true
	} else if kerrors.IsNotFound(err) {
		err = newClient.Get(ctx, client.ObjectKey{Namespace: types.DefaultKubeVelaNS, Name: capabilityName}, &policyDef)
		if err == nil {
			foundCapability = true
		}
	}
	if foundCapability {
		capability, err = GetCapabilityByPolicyDefinitionObject(policyDef)
		if err != nil {
			return nil, err
		}
		return capability, nil
	}

	var sourceDef v1beta1.SourceDefinition
	err = newClient.Get(ctx, client.ObjectKey{Namespace: ns, Name: capabilityName}, &sourceDef)
	if err == nil {
		foundCapability = true
	} else if kerrors.IsNotFound(err) {
		err = newClient.Get(ctx, client.ObjectKey{Namespace: types.DefaultKubeVelaNS, Name: capabilityName}, &sourceDef)
		if err == nil {
			foundCapability = true
		}
	}
	if foundCapability {
		capability, err = GetCapabilityBySourceDefinitionObject(sourceDef)
		if err != nil {
			return nil, err
		}
		return capability, nil
	}

	if ns == types.DefaultKubeVelaNS {
		return nil, fmt.Errorf("could not find %s in namespace %s", capabilityName, ns)
	}
	return nil, fmt.Errorf("could not find %s in namespace %s, or %s", capabilityName, ns, types.DefaultKubeVelaNS)
}

// GetCapabilityFromDefinitionRevision gets capabilities from the underlying Definition in DefinitionRevisions
func GetCapabilityFromDefinitionRevision(ctx context.Context, c common.Args, ns, defName string, r int64) (*types.Capability, error) {
	k8sClient, err := c.GetClient()
	if err != nil {
		return nil, err
	}

	revs, err := definition.SearchDefinitionRevisions(ctx, k8sClient, ns, defName, "", r)
	if err != nil {
		return nil, err
	}
	// `ns` defaults to `default` in `vela show`, if user doesn't specify anything,
	// which often is not the desired behavior.
	// So we need to search again in the vela-system namespace, if no revisions found.
	// This behavior is consistent with the code above in GetCapabilityByName(), which also does double-search.
	if len(revs) == 0 && ns == "default" {
		revs, err = definition.SearchDefinitionRevisions(ctx, k8sClient, types.DefaultKubeVelaNS, defName, "", r)
		if err != nil {
			return nil, err
		}
	}
	if len(revs) == 0 {
		return nil, fmt.Errorf("no %s with revision %d found in namespace %s or %s", defName, r, ns, types.DefaultKubeVelaNS)
	}

	rev := revs[0]

	switch rev.Spec.DefinitionType {
	case commontypes.ComponentType:
		var refName string
		componentDef := rev.Spec.ComponentDefinition
		// if workload type of ComponentDefinition is unclear,
		// set the DefinitionReference's Name to AutoDetectWorkloadDefinition
		if componentDef.Spec.Workload.Type == types.AutoDetectWorkloadDefinition {
			refName = types.AutoDetectWorkloadDefinition
		} else {
			ref, err := util.ConvertWorkloadGVK2Definition(k8sClient.RESTMapper(), componentDef.Spec.Workload.Definition)
			if err != nil {
				return nil, err
			}
			refName = ref.Name
		}
		return GetCapabilityByComponentDefinitionObject(componentDef, refName)
	case commontypes.TraitType:
		return GetCapabilityByTraitDefinitionObject(rev.Spec.TraitDefinition)
	case commontypes.WorkflowStepType:
		return GetCapabilityByWorkflowStepDefinitionObject(rev.Spec.WorkflowStepDefinition)
	case commontypes.PolicyType:
		return GetCapabilityByPolicyDefinitionObject(rev.Spec.PolicyDefinition)
	case commontypes.SourceType:
		return GetCapabilityBySourceDefinitionObject(rev.Spec.SourceDefinition)
	default:
		return nil, fmt.Errorf("unsupported type %s", rev.Spec.DefinitionType)
	}
}

// GetCapabilityByComponentDefinitionObject gets capability by ComponentDefinition object
func GetCapabilityByComponentDefinitionObject(componentDef v1beta1.ComponentDefinition, referenceName string) (*types.Capability, error) {
	capability, err := HandleDefinition(componentDef.Name, referenceName, componentDef.Annotations, componentDef.Labels,
		componentDef.Spec.Extension, types.TypeComponentDefinition, nil, componentDef.Spec.Schematic)
	if err != nil {
		return nil, errors.Wrap(err, "failed to handle ComponentDefinition")
	}
	capability.Namespace = componentDef.Namespace
	return &capability, nil
}

// GetCapabilityByTraitDefinitionObject gets capability by TraitDefinition object
func GetCapabilityByTraitDefinitionObject(traitDef v1beta1.TraitDefinition) (*types.Capability, error) {
	var (
		capability types.Capability
		err        error
	)
	capability, err = HandleDefinition(traitDef.Name, traitDef.Spec.Reference.Name, traitDef.Annotations, traitDef.Labels,
		traitDef.Spec.Extension, types.TypeTrait, nil, traitDef.Spec.Schematic)
	if err != nil {
		return nil, errors.Wrap(err, "failed to handle TraitDefinition")
	}
	capability.Namespace = traitDef.Namespace
	return &capability, nil
}

// GetCapabilityByWorkflowStepDefinitionObject gets capability by WorkflowStepDefinition object
func GetCapabilityByWorkflowStepDefinitionObject(wfStepDef v1beta1.WorkflowStepDefinition) (*types.Capability, error) {
	capability, err := HandleDefinition(wfStepDef.Name, wfStepDef.Spec.Reference.Name, wfStepDef.Annotations, wfStepDef.Labels,
		nil, types.TypeWorkflowStep, nil, wfStepDef.Spec.Schematic)
	if err != nil {
		return nil, errors.Wrap(err, "failed to handle WorkflowStepDefinition")
	}
	capability.Namespace = wfStepDef.Namespace
	return &capability, nil
}

// GetCapabilityByPolicyDefinitionObject gets capability by PolicyDefinition object
func GetCapabilityByPolicyDefinitionObject(def v1beta1.PolicyDefinition) (*types.Capability, error) {
	capability, err := HandleDefinition(def.Name, def.Spec.Reference.Name, def.Annotations, def.Labels,
		nil, types.TypePolicy, nil, def.Spec.Schematic)
	if err != nil {
		return nil, errors.Wrap(err, "failed to handle PolicyDefinition")
	}
	capability.Namespace = def.Namespace
	return &capability, nil
}

// GetCapabilityBySourceDefinitionObject gets capability by SourceDefinition object
func GetCapabilityBySourceDefinitionObject(def v1beta1.SourceDefinition) (*types.Capability, error) {
	// SourceDefinitionSpec has no Reference field, so the CRD reference name is empty.
	capability, err := HandleDefinition(def.Name, "", def.Annotations, def.Labels,
		nil, types.TypeSource, nil, def.Spec.Schematic)
	if err != nil {
		return nil, errors.Wrap(err, "failed to handle SourceDefinition")
	}
	capability.Namespace = def.Namespace
	// Surface the source-specific output contract (schema:) and caching (storage:)
	// so `vela def show` can tell users what data the source provides and how it
	// is cached. Failures here are non-fatal: the capability is still usable.
	if capability.CueTemplate != "" {
		if outputs, err := extractSourceOutputs(capability.CueTemplate); err != nil {
			klog.Warningf("parse source outputs for %s: %v", def.Name, err)
		} else {
			capability.SourceOutputs = outputs
		}
		if storage, err := extractStorageFields(capability.CueTemplate); err != nil {
			klog.Warningf("parse source storage for %s: %v", def.Name, err)
		} else {
			capability.SourceStorage = storage
		}
		// The cache key moved into $internal: when it became generated rather than
		// authored, and this extractor read only storage: - so `vela def show`
		// stopped printing the one field an operator needs to correlate a source
		// with its cache entry. Prepended rather than appended: the key is the
		// identity, the TTL is a policy about it.
		capability.SourceStorage = append(internalCacheFields(capability.CueTemplate), capability.SourceStorage...)

		capability.SourceSurfaces = sourceSurfaces(capability.CueTemplate)
	}
	return &capability, nil
}

// internalCacheFields reads the generated $internal: block - the cache key and
// the context fields it is built from.
//
// Non-fatal on any failure, like every other extractor here: a definition that
// predates the generated block simply has nothing to show.
func internalCacheFields(template string) []types.SourceStorageField {
	fields, err := extractBlockFields(template, "$internal")
	if err != nil {
		klog.Warningf("parse source $internal block: %v", err)
		return nil
	}
	var out []types.SourceStorageField
	for _, f := range fields {
		switch f.Name {
		case "key":
			out = append(out, f)
		case "keyInputs":
			out = append(out, types.SourceStorageField{
				Name: "keyInputs", Value: formatKeyInputs(f.Value)})
		}
	}
	return out
}

// formatKeyInputs renders the generated keyInputs list as the context reads an
// author would recognise: `["cluster","namespace"]` becomes
// `context.cluster, context.namespace`.
//
// The stored form is a CUE list because that is what the generator writes and
// what admission re-derives; it is not what anyone wants to read in a table.
func formatKeyInputs(raw string) string {
	trimmed := strings.TrimSpace(raw)
	trimmed = strings.TrimPrefix(trimmed, "[")
	trimmed = strings.TrimSuffix(trimmed, "]")
	var names []string
	for _, part := range strings.Split(trimmed, ",") {
		part = strings.Trim(strings.TrimSpace(part), `"`)
		if part != "" {
			names = append(names, "context."+part)
		}
	}
	if len(names) == 0 {
		// Not a gap: a source that reads no context resolves to one entry shared
		// by the whole cluster, which is worth stating rather than leaving blank.
		return "(none - one cache entry for the whole cluster)"
	}
	return strings.Join(names, ", ")
}

// sourceSurfaces reports, for every surface, whether this source can be consumed
// there and why not when it cannot.
//
// Derived, never authored. A source is restricted by the context its template
// reads - one keyed on context.componentName cannot resolve in a workflow step,
// because no component is being rendered there - and optionally narrowed further
// by the definition's own consumableFrom. Both are already enforced at admission;
// the only thing missing was telling the author before an Application is rejected.
//
// Every surface is returned, the reachable ones included, so the caller can show a
// row each: "workflow steps: no, reads context.componentName" answers a question
// that a list quietly omitting workflow steps does not.
func sourceSurfaces(template string) []types.SourceSurface {
	fields, err := cachekey.RequiredContext(template)
	if err != nil {
		klog.Warningf("infer source context reads: %v", err)
		return nil
	}

	// Absent consumableFrom means every surface; present means only those named.
	declared, derr := sourcedefinition.ParseConsumableFrom(template)
	if derr != nil {
		klog.Warningf("parse source consumableFrom: %v", derr)
		declared = nil
	}

	out := make([]types.SourceSurface, 0, len(sources.ConsumableSurfaces))
	for _, surface := range sources.ConsumableSurfaces {
		row := types.SourceSurface{Name: propexpr.SurfacePlural(surface), Consumable: true}

		// The two exclusions are reported together because they are independent
		// and have different fixes: what a template reads can only change by
		// changing the reads, while consumableFrom is a deliberate choice.
		var reasons []string
		if missing := cachekey.MissingOn(fields, surface); len(missing) > 0 {
			reasons = append(reasons, "reads "+strings.Join(missing, ", "))
		}
		if len(declared) > 0 && !slices.Contains(declared, surface) {
			reasons = append(reasons, "not in consumableFrom")
		}
		if len(reasons) > 0 {
			row.Consumable, row.Reason = false, strings.Join(reasons, "; ")
		}
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// extractSourceOutputs parses the `schema:` block of a SourceDefinition template
// into displayable parameters. The schema block is a plain set of type
// declarations (e.g. `value: int`), so it compiles standalone once relabeled as
// a parameter block and reuses the same extractor as inputs.
func extractSourceOutputs(template string) ([]types.Parameter, error) {
	schema, err := extractTopLevelCUEBlock(template, "schema")
	if err != nil || schema == "" {
		return nil, err
	}
	params, err := cue.GetParameters("parameter: " + schema)
	if err != nil {
		if errors.Is(err, cue.ErrParameterNotExist) {
			return nil, nil
		}
		return nil, err
	}
	return params, nil
}

// extractStorageFields parses the `storage:` block of a SourceDefinition
// template into ordered name/value pairs (storageTTL, onStaleFailure, ...).
// Values are the authored CUE expressions kept verbatim; interpolations like
// \(parameter.min) are NOT evaluated.
func extractStorageFields(template string) ([]types.SourceStorageField, error) {
	return extractBlockFields(template, "storage")
}

// extractBlockFields returns the named top-level block's fields as ordered
// name/value pairs. Used for both the authored `storage:` block and the
// generated `$internal:` one, which have the same shape and are read the same
// way - the difference between them is who writes them, not how they parse.
func extractBlockFields(template, blockName string) ([]types.SourceStorageField, error) {
	file, err := parser.ParseFile("-", template, parser.ParseComments)
	if err != nil {
		return nil, err
	}
	for _, decl := range file.Decls {
		field, ok := decl.(*ast.Field)
		if !ok {
			continue
		}
		name, _, err := ast.LabelName(field.Label)
		if err != nil || name != blockName {
			continue
		}
		lit, ok := field.Value.(*ast.StructLit)
		if !ok {
			return nil, nil
		}
		var fields []types.SourceStorageField
		for _, el := range lit.Elts {
			sub, ok := el.(*ast.Field)
			if !ok {
				continue
			}
			subName, _, err := ast.LabelName(sub.Label)
			if err != nil {
				continue
			}
			bt, err := format.Node(sub.Value)
			if err != nil {
				return nil, err
			}
			fields = append(fields, types.SourceStorageField{
				Name:  subName,
				Value: strings.Trim(string(bt), `"`),
			})
		}
		return fields, nil
	}
	return nil, nil
}

// extractTopLevelCUEBlock returns the formatted value of the named top-level
// field from a CUE template (e.g. "schema"), or "" if absent.
func extractTopLevelCUEBlock(template, fieldName string) (string, error) {
	file, err := parser.ParseFile("-", template, parser.ParseComments)
	if err != nil {
		return "", err
	}
	for _, decl := range file.Decls {
		field, ok := decl.(*ast.Field)
		if !ok {
			continue
		}
		name, _, err := ast.LabelName(field.Label)
		if err != nil || name != fieldName {
			continue
		}
		bt, err := format.Node(field.Value)
		if err != nil {
			return "", err
		}
		return string(bt), nil
	}
	return "", nil
}

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
	"strings"

	"github.com/pkg/errors"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kubevela/workflow/pkg/cue/packages"

	commontypes "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/appfile"
	"github.com/oam-dev/kubevela/pkg/cue"
	"github.com/oam-dev/kubevela/pkg/definition"
	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/pkg/utils"
	"github.com/oam-dev/kubevela/pkg/utils/common"
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
	}

	if len(capabilities) > 0 {
		return capabilities, nil
	}
	return nil, fmt.Errorf("could not find any components, traits or workflowSteps from namespace %s and %s", namespace, types.DefaultKubeVelaNS)
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
		dm, err := c.GetDiscoveryMapper()
		if err != nil {
			return nil, nil, err
		}

		defRef := commontypes.DefinitionReference{
			Name: cd.Spec.Workload.Type,
		}
		if cd.Spec.Workload.Type != types.AutoDetectWorkloadDefinition {
			defRef, err = util.ConvertWorkloadGVK2Definition(dm, cd.Spec.Workload.Definition)
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
			if err = validateCapabilities(tmp, dm, cd.Name, defRef); err != nil {
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
	config, err := c.GetConfig()
	if err != nil {
		return nil, nil, err
	}
	dm, err := discoverymapper.New(config)
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
			if err = validateCapabilities(tmp, dm, td.Name, td.Spec.Reference); err != nil {
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

	config, err := c.GetConfig()
	if err != nil {
		return nil, nil, err
	}
	pd, err := packages.NewPackageDiscover(config)
	if err != nil {
		return nil, nil, err
	}

	var templateErrors []error
	for _, def := range workflowStepDefs.Items {
		tmp, err := GetCapabilityByWorkflowStepDefinitionObject(def, pd)
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
		tmp, err := GetCapabilityByPolicyDefinitionObject(def, nil)
		if err != nil {
			templateErrors = append(templateErrors, err)
			continue
		}
		templates = append(templates, *tmp)
	}
	return templates, templateErrors, nil
}

// validateCapabilities validates whether GVK are successfully retrieved.
func validateCapabilities(tmp *types.Capability, dm discoverymapper.DiscoveryMapper, definitionName string, reference commontypes.DefinitionReference) error {
	var err error
	gvk, err := util.GetGVKFromDefinition(dm, reference)
	if err != nil {
		errMsg := err.Error()
		var substr = "no matches for "
		if strings.Contains(errMsg, substr) {
			err = fmt.Errorf("expected provider: %s", strings.Split(errMsg, substr)[1])
		}
		return fmt.Errorf("installing capability '%s'... %w", definitionName, err)
	}
	tmp.CrdInfo = &types.CRDInfo{
		APIVersion: metav1.GroupVersion{Group: gvk.Group, Version: gvk.Version}.String(),
		Kind:       gvk.Kind,
	}

	return nil
}

// HandleDefinition will handle definition to capability
func HandleDefinition(name, crdName string, annotation, labels map[string]string, extension *runtime.RawExtension, tp types.CapType,
	applyTo []string, schematic *commontypes.Schematic, pd *packages.PackageDiscover) (types.Capability, error) {
	var tmp types.Capability
	tmp, err := HandleTemplate(extension, schematic, name, pd)
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
func HandleTemplate(in *runtime.RawExtension, schematic *commontypes.Schematic, name string, pd *packages.PackageDiscover) (types.Capability, error) {
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
		if schematic.KUBE != nil {
			tmp.Category = types.KubeCategory
			tmp.KubeTemplate = schematic.KUBE.Template
			tmp.KubeParameter = schematic.KUBE.Parameters
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
		if schematic != nil && schematic.HELM != nil {
			tmp.Category = types.HelmCategory
			return tmp, nil
		}
		return types.Capability{}, errors.New("template not exist in definition")
	}
	tmp.Parameters, err = cue.GetParameters(tmp.CueTemplate, pd)
	if err != nil && !errors.Is(err, cue.ErrParameterNotExist) {
		return types.Capability{}, err
	}
	tmp.Category = types.CUECategory
	return tmp, nil
}

// GetCapabilityByName gets capability by definition name
func GetCapabilityByName(ctx context.Context, c common.Args, capabilityName string, ns string, pd *packages.PackageDiscover) (*types.Capability, error) {
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
			dm, err := c.GetDiscoveryMapper()
			if err != nil {
				return nil, err
			}
			ref, err := util.ConvertWorkloadGVK2Definition(dm, componentDef.Spec.Workload.Definition)
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
		capability, err = GetCapabilityByWorkflowStepDefinitionObject(wfStepDef, pd)
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
func GetCapabilityFromDefinitionRevision(ctx context.Context, c common.Args, pd *packages.PackageDiscover, ns, defName string, r int64) (*types.Capability, error) {
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
			dm, err := c.GetDiscoveryMapper()
			if err != nil {
				return nil, err
			}
			ref, err := util.ConvertWorkloadGVK2Definition(dm, componentDef.Spec.Workload.Definition)
			if err != nil {
				return nil, err
			}
			refName = ref.Name
		}
		return GetCapabilityByComponentDefinitionObject(componentDef, refName)
	case commontypes.TraitType:
		return GetCapabilityByTraitDefinitionObject(rev.Spec.TraitDefinition)
	case commontypes.WorkflowStepType:
		return GetCapabilityByWorkflowStepDefinitionObject(rev.Spec.WorkflowStepDefinition, pd)
	default:
		return nil, fmt.Errorf("unsupported type %s", rev.Spec.DefinitionType)
	}
}

// GetCapabilityByComponentDefinitionObject gets capability by ComponentDefinition object
func GetCapabilityByComponentDefinitionObject(componentDef v1beta1.ComponentDefinition, referenceName string) (*types.Capability, error) {
	capability, err := HandleDefinition(componentDef.Name, referenceName, componentDef.Annotations, componentDef.Labels,
		componentDef.Spec.Extension, types.TypeComponentDefinition, nil, componentDef.Spec.Schematic, nil)
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
		traitDef.Spec.Extension, types.TypeTrait, nil, traitDef.Spec.Schematic, nil)
	if err != nil {
		return nil, errors.Wrap(err, "failed to handle TraitDefinition")
	}
	capability.Namespace = traitDef.Namespace
	return &capability, nil
}

// GetCapabilityByWorkflowStepDefinitionObject gets capability by WorkflowStepDefinition object
func GetCapabilityByWorkflowStepDefinitionObject(wfStepDef v1beta1.WorkflowStepDefinition, pd *packages.PackageDiscover) (*types.Capability, error) {
	capability, err := HandleDefinition(wfStepDef.Name, wfStepDef.Spec.Reference.Name, wfStepDef.Annotations, wfStepDef.Labels,
		nil, types.TypeWorkflowStep, nil, wfStepDef.Spec.Schematic, pd)
	if err != nil {
		return nil, errors.Wrap(err, "failed to handle WorkflowStepDefinition")
	}
	capability.Namespace = wfStepDef.Namespace
	return &capability, nil
}

// GetCapabilityByPolicyDefinitionObject gets capability by PolicyDefinition object
func GetCapabilityByPolicyDefinitionObject(def v1beta1.PolicyDefinition, pd *packages.PackageDiscover) (*types.Capability, error) {
	capability, err := HandleDefinition(def.Name, def.Spec.Reference.Name, def.Annotations, def.Labels,
		nil, types.TypePolicy, nil, def.Spec.Schematic, pd)
	if err != nil {
		return nil, errors.Wrap(err, "failed to handle PolicyDefinition")
	}
	capability.Namespace = def.Namespace
	return &capability, nil
}

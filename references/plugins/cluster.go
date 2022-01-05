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

package plugins

import (
	"context"
	"fmt"
	"os"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/pkg/errors"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	commontypes "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/appfile"
	"github.com/oam-dev/kubevela/pkg/cue"
	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	"github.com/oam-dev/kubevela/pkg/utils/helm"
	util2 "github.com/oam-dev/kubevela/pkg/utils/util"
)

// DescriptionUndefined indicates the description is not defined
const DescriptionUndefined = "description not defined"

// GetCapabilitiesFromCluster will get capability from K8s cluster
func GetCapabilitiesFromCluster(ctx context.Context, namespace string, c common.Args, selector labels.Selector) ([]types.Capability, error) {
	workloads, _, err := GetComponentsFromCluster(ctx, namespace, c, selector)
	if err != nil {
		return nil, err
	}
	traits, _, err := GetTraitsFromCluster(ctx, namespace, c, selector)
	if err != nil {
		return nil, err
	}
	workloads = append(workloads, traits...)
	return workloads, nil
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

	// get components from default namespace
	if workloads, _, err := GetComponentsFromClusterWithValidateOption(ctx, types.DefaultKubeVelaNS, c, selector, false); err == nil {
		capabilities = append(capabilities, workloads...)
	}

	// get traits from default namespace
	if traits, _, err := GetTraitsFromClusterWithValidateOption(ctx, types.DefaultKubeVelaNS, c, selector, false); err == nil {
		capabilities = append(capabilities, traits...)
	}

	if len(capabilities) > 0 {
		return capabilities, nil
	}
	return nil, fmt.Errorf("could not find any components or traits from namespace %s and %s", namespace, types.DefaultKubeVelaNS)
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
	dm, err := discoverymapper.New(c.Config)
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
		tmp, err := GetCapabilityByTraitDefinitionObject(td)
		if err != nil {
			templateErrors = append(templateErrors, errors.Wrapf(err, "handle trait template `%s` failed", td.Name))
			continue
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

// validateCapabilities validates whether helm charts are successful installed, GVK are successfully retrieved.
func validateCapabilities(tmp *types.Capability, dm discoverymapper.DiscoveryMapper, definitionName string, reference commontypes.DefinitionReference) error {
	var err error
	if tmp.Install != nil {
		tmp.Source = &types.Source{ChartName: tmp.Install.Helm.Name}
		ioStream := util2.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr}
		if err = helm.InstallHelmChart(ioStream, tmp.Install.Helm); err != nil {
			return fmt.Errorf("unable to install helm chart dependency %s(%s from %s) for this trait '%s': %w ", tmp.Install.Helm.Name, tmp.Install.Helm.Version, tmp.Install.Helm.URL, definitionName, err)
		}
	}
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
		if schematic.KUBE != nil {
			tmp.Category = types.KubeCategory
			tmp.KubeTemplate = schematic.KUBE.Template
			tmp.KubeParameter = schematic.KUBE.Parameters
			return tmp, nil
		}
	}
	if tmp.CueTemplateURI != "" {
		b, err := common.HTTPGet(context.Background(), tmp.CueTemplateURI)
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
	tmp.Parameters, err = cue.GetParameters(tmp.CueTemplate)
	if err != nil {
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
	return nil, fmt.Errorf("could not find %s in namespace %s, or %s", capabilityName, ns, types.DefaultKubeVelaNS)
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

package plugins

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	corev1alpha2 "github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/apis/types"
	util2 "github.com/oam-dev/kubevela/pkg/commands/util"
	"github.com/oam-dev/kubevela/pkg/cue"
	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	"github.com/oam-dev/kubevela/pkg/utils/helm"
	"github.com/oam-dev/kubevela/pkg/utils/system"
)

// DescriptionUndefined indicates the description is not defined
const DescriptionUndefined = "description not defined"

// GetCapabilitiesFromCluster will get capability from K8s cluster
func GetCapabilitiesFromCluster(ctx context.Context, namespace string, c types.Args, syncDir string, selector labels.Selector) ([]types.Capability, error) {
	workloads, _, err := GetWorkloadsFromCluster(ctx, namespace, c, syncDir, selector)
	if err != nil {
		return nil, err
	}
	traits, _, err := GetTraitsFromCluster(ctx, namespace, c, syncDir, selector)
	if err != nil {
		return nil, err
	}
	workloads = append(workloads, traits...)
	return workloads, nil
}

// GetWorkloadsFromCluster will get capability from K8s cluster
func GetWorkloadsFromCluster(ctx context.Context, namespace string, c types.Args, syncDir string, selector labels.Selector) ([]types.Capability, []error, error) {
	newClient, err := client.New(c.Config, client.Options{Scheme: c.Schema})
	if err != nil {
		return nil, nil, err
	}
	dm, err := discoverymapper.New(c.Config)
	if err != nil {
		return nil, nil, err
	}

	var templates []types.Capability
	var workloadDefs corev1alpha2.WorkloadDefinitionList
	err = newClient.List(ctx, &workloadDefs, &client.ListOptions{Namespace: namespace, LabelSelector: selector})
	if err != nil {
		return nil, nil, fmt.Errorf("list WorkloadDefinition err: %w", err)
	}

	var templateErrors []error
	for _, wd := range workloadDefs.Items {
		tmp, err := HandleDefinition(wd.Name, syncDir, wd.Spec.Reference.Name, wd.Annotations, wd.Spec.Extension, types.TypeWorkload, nil, wd.Spec.Template)
		if err != nil {
			templateErrors = append(templateErrors, errors.Wrapf(err, "handle workload template `%s` failed", wd.Name))
			continue
		}
		if tmp, err = validateCapabilities(tmp, dm, wd.Name, wd.Spec.Reference); err != nil {
			return nil, nil, err
		}
		templates = append(templates, tmp)
	}
	return templates, templateErrors, nil
}

// GetTraitsFromCluster will get capability from K8s cluster
func GetTraitsFromCluster(ctx context.Context, namespace string, c types.Args, syncDir string, selector labels.Selector) ([]types.Capability, []error, error) {
	newClient, err := client.New(c.Config, client.Options{Scheme: c.Schema})
	if err != nil {
		return nil, nil, err
	}
	dm, err := discoverymapper.New(c.Config)
	if err != nil {
		return nil, nil, err
	}
	var templates []types.Capability
	var traitDefs corev1alpha2.TraitDefinitionList
	err = newClient.List(ctx, &traitDefs, &client.ListOptions{Namespace: namespace, LabelSelector: selector})
	if err != nil {
		return nil, nil, fmt.Errorf("list TraitDefinition err: %w", err)
	}

	var templateErrors []error
	for _, td := range traitDefs.Items {
		tmp, err := HandleDefinition(td.Name, syncDir, td.Spec.Reference.Name, td.Annotations, td.Spec.Extension, types.TypeTrait, td.Spec.AppliesToWorkloads, td.Spec.Template)
		if err != nil {
			templateErrors = append(templateErrors, errors.Wrapf(err, "handle trait template `%s` failed", td.Name))
			continue
		}
		if tmp, err = validateCapabilities(tmp, dm, td.Name, td.Spec.Reference); err != nil {
			return nil, nil, err
		}
		templates = append(templates, tmp)
	}
	return templates, templateErrors, nil
}

// validateCapabilities validates whether helm charts are successful installed, GVK are successfully retrieved.
func validateCapabilities(tmp types.Capability, dm discoverymapper.DiscoveryMapper, definitionName string, reference corev1alpha2.DefinitionReference) (types.Capability, error) {
	var err error
	if tmp.Install != nil {
		tmp.Source = &types.Source{ChartName: tmp.Install.Helm.Name}
		ioStream := util2.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr}
		if err = helm.InstallHelmChart(ioStream, tmp.Install.Helm); err != nil {
			return tmp, fmt.Errorf("unable to install helm chart dependency %s(%s from %s) for this trait '%s': %w ", tmp.Install.Helm.Name, tmp.Install.Helm.Version, tmp.Install.Helm.URL, definitionName, err)
		}
	}
	gvk, err := util.GetGVKFromDefinition(dm, reference)
	if err != nil {
		errMsg := err.Error()
		var substr = "no matches for "
		if strings.Contains(errMsg, substr) {
			err = fmt.Errorf("expected provider: %s", strings.Split(errMsg, substr)[1])
		}
		return tmp, fmt.Errorf("installing capability '%s'... %w", definitionName, err)
	}
	tmp.CrdInfo = &types.CRDInfo{
		APIVersion: gvk.GroupVersion().String(),
		Kind:       gvk.Kind,
	}

	return tmp, nil
}

// HandleDefinition will handle definition to capability
func HandleDefinition(name, syncDir, crdName string, annotation map[string]string, extension *runtime.RawExtension, tp types.CapType, applyTo []string, template string) (types.Capability, error) {
	var tmp types.Capability
	tmp, err := HandleTemplate(extension, template, name, syncDir)
	if err != nil {
		return types.Capability{}, err
	}
	tmp.Type = tp
	if tp == types.TypeTrait {
		tmp.AppliesTo = applyTo
	}
	tmp.CrdName = crdName
	tmp.Description = GetDescription(annotation)
	return tmp, nil
}

// GetDescription get description from annotation
func GetDescription(annotation map[string]string) string {
	if annotation == nil {
		return DescriptionUndefined
	}
	desc, ok := annotation[types.AnnDescription]
	if !ok {
		return DescriptionUndefined
	}
	return desc
}

// HandleTemplate will handle definition template to capability
func HandleTemplate(in *runtime.RawExtension, specTemplate, name, syncDir string) (types.Capability, error) {
	tmp, err := util.ConvertTemplateJSON2Object(in, specTemplate)
	if err != nil {
		return types.Capability{}, err
	}
	tmp.Name = name
	// if spec.template is not empty it should has the highest priority
	if specTemplate != "" {
		tmp.CueTemplate = specTemplate
		tmp.CueTemplateURI = ""
	}
	if tmp.CueTemplateURI != "" {
		b, err := common.HTTPGet(context.Background(), tmp.CueTemplateURI)
		if err != nil {
			return types.Capability{}, err
		}
		tmp.CueTemplate = string(b)
	}
	if tmp.CueTemplate == "" {
		return types.Capability{}, errors.New("template not exist in definition")
	}
	_, _ = system.CreateIfNotExist(syncDir)
	filePath := filepath.Join(syncDir, name+".cue")
	//nolint:gosec
	err = ioutil.WriteFile(filePath, []byte(tmp.CueTemplate), 0644)
	if err != nil {
		return types.Capability{}, err
	}
	tmp.DefinitionPath = filePath
	tmp.Parameters, err = cue.GetParameters(filePath)
	if err != nil {
		return types.Capability{}, err
	}
	return tmp, nil
}

// SyncDefinitionsToLocal sync definitions to local
func SyncDefinitionsToLocal(ctx context.Context, c types.Args, localDefinitionDir string) ([]types.Capability, []string, error) {
	var syncedTemplates []types.Capability
	var warnings []string

	templates, templateErrors, err := GetWorkloadsFromCluster(ctx, types.DefaultKubeVelaNS, c, localDefinitionDir, nil)
	if err != nil {
		return nil, nil, err
	}
	if len(templateErrors) > 0 {
		for _, e := range templateErrors {
			warnings = append(warnings, fmt.Sprintf("WARN: %v, you will unable to use this workload capability\n", e))
		}
	}
	syncedTemplates = append(syncedTemplates, templates...)
	SinkTemp2Local(templates, localDefinitionDir)

	templates, templateErrors, err = GetTraitsFromCluster(ctx, types.DefaultKubeVelaNS, c, localDefinitionDir, nil)
	if err != nil {
		return nil, warnings, err
	}
	if len(templateErrors) > 0 {
		for _, e := range templateErrors {
			warnings = append(warnings, fmt.Sprintf("WARN: %v, you will unable to use this trait capability\n", e))
		}
	}
	syncedTemplates = append(syncedTemplates, templates...)
	SinkTemp2Local(templates, localDefinitionDir)
	return syncedTemplates, warnings, nil
}

// SyncDefinitionToLocal sync definitions to local
func SyncDefinitionToLocal(ctx context.Context, c types.Args, localDefinitionDir string, capabilityName string) (*types.Capability, error) {
	var foundCapability bool

	newClient, err := client.New(c.Config, client.Options{Scheme: c.Schema})
	if err != nil {
		return nil, err
	}
	var workloadDef corev1alpha2.WorkloadDefinition
	err = newClient.Get(ctx, client.ObjectKey{Namespace: types.DefaultKubeVelaNS, Name: capabilityName}, &workloadDef)
	if err == nil {
		// return nil, fmt.Errorf("get WorkloadDefinition err: %w", err)
		foundCapability = true
	}
	if foundCapability {
		template, err := HandleDefinition(capabilityName, localDefinitionDir, workloadDef.Spec.Reference.Name,
			workloadDef.Annotations, workloadDef.Spec.Extension, types.TypeWorkload, nil, workloadDef.Spec.Template)
		if err == nil {
			return &template, nil
		}
	}

	foundCapability = false
	var traitDef corev1alpha2.TraitDefinition
	err = newClient.Get(ctx, client.ObjectKey{Namespace: types.DefaultKubeVelaNS, Name: capabilityName}, &traitDef)
	if err == nil {
		foundCapability = true
	}
	if foundCapability {
		template, err := HandleDefinition(capabilityName, localDefinitionDir, traitDef.Spec.Reference.Name,
			traitDef.Annotations, traitDef.Spec.Extension, types.TypeTrait, nil, workloadDef.Spec.Template)
		if err == nil {
			return &template, nil
		}
	}
	return nil, fmt.Errorf("%s is not a valid workload type or trait", capabilityName)
}

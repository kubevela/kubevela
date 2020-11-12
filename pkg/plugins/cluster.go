package plugins

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"path/filepath"

	"github.com/crossplane/oam-kubernetes-runtime/pkg/oam/util"

	"github.com/crossplane/oam-kubernetes-runtime/pkg/oam/discoverymapper"

	"github.com/oam-dev/kubevela/api/types"
	"github.com/oam-dev/kubevela/pkg/cue"
	"github.com/oam-dev/kubevela/pkg/utils/system"
	"github.com/pkg/errors"

	corev1alpha2 "github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const DescriptionUndefined = "description not defined"

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
		return nil, nil, fmt.Errorf("list WorkloadDefinition err: %s", err)
	}

	var templateErrors []error
	for _, wd := range workloadDefs.Items {
		tmp, err := HandleDefinition(wd.Name, syncDir, wd.Spec.Reference.Name, wd.Annotations, wd.Spec.Extension, types.TypeWorkload, nil)
		if err != nil {
			templateErrors = append(templateErrors, errors.Wrapf(err, "handle workload template `%s` failed", wd.Name))
			continue
		}
		gvk, err := util.GetGVKFromDefinition(dm, wd.Spec.Reference)
		if err != nil {
			return nil, nil, err
		}
		tmp.CrdInfo = &types.CrdInfo{
			APIVersion: gvk.GroupVersion().String(),
			Kind:       gvk.Kind,
		}
		templates = append(templates, tmp)
	}
	return templates, templateErrors, nil
}

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
		return nil, nil, fmt.Errorf("list TraitDefinition err: %s", err)
	}

	var templateErrors []error
	for _, td := range traitDefs.Items {
		tmp, err := HandleDefinition(td.Name, syncDir, td.Spec.Reference.Name, td.Annotations, td.Spec.Extension, types.TypeTrait, td.Spec.AppliesToWorkloads)
		if err != nil {
			templateErrors = append(templateErrors, errors.Wrapf(err, "handle trait template `%s` failed\n", td.Name))
			continue
		}
		gvk, err := util.GetGVKFromDefinition(dm, td.Spec.Reference)
		if err != nil {
			return nil, nil, err
		}
		tmp.CrdInfo = &types.CrdInfo{
			APIVersion: gvk.GroupVersion().String(),
			Kind:       gvk.Kind,
		}
		templates = append(templates, tmp)
	}
	return templates, templateErrors, nil
}

func HandleDefinition(name, syncDir, crdName string, annotation map[string]string, extension *runtime.RawExtension, tp types.CapType, applyTo []string) (types.Capability, error) {
	var tmp types.Capability
	tmp, err := HandleTemplate(extension, name, syncDir)
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

func HandleTemplate(in *runtime.RawExtension, name, syncDir string) (types.Capability, error) {
	tmp, err := types.ConvertTemplateJSON2Object(in)
	if err != nil {
		return types.Capability{}, err
	}
	tmp.Name = name

	var cueTemplate string
	if tmp.CueTemplateURI != "" {
		res, err := http.Get(tmp.CueTemplateURI)
		if err != nil {
			return types.Capability{}, err
		}
		defer res.Body.Close()
		b, err := ioutil.ReadAll(res.Body)
		if err != nil {
			return types.Capability{}, err
		}
		cueTemplate = string(b)
		tmp.CueTemplate = cueTemplate
	} else {
		if tmp.CueTemplate == "" {
			return types.Capability{}, errors.New("template not exist in definition")
		}
		cueTemplate = tmp.CueTemplate
	}
	_, _ = system.CreateIfNotExist(syncDir)
	filePath := filepath.Join(syncDir, name+".cue")
	err = ioutil.WriteFile(filePath, []byte(cueTemplate), 0644)
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

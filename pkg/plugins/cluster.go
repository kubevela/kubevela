package plugins

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"

	corev1alpha2 "github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
	"github.com/crossplane/oam-kubernetes-runtime/pkg/oam/discoverymapper"
	"github.com/crossplane/oam-kubernetes-runtime/pkg/oam/util"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/api/types"
	util2 "github.com/oam-dev/kubevela/pkg/commands/util"
	"github.com/oam-dev/kubevela/pkg/cue"
	"github.com/oam-dev/kubevela/pkg/utils/helm"
	"github.com/oam-dev/kubevela/pkg/utils/system"
)

// nolint
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
		return nil, nil, fmt.Errorf("list WorkloadDefinition err: %s", err)
	}

	var templateErrors []error
	for _, wd := range workloadDefs.Items {
		tmp, err := HandleDefinition(wd.Name, syncDir, wd.Spec.Reference.Name, wd.Annotations, wd.Spec.Extension, types.TypeWorkload, nil)
		if err != nil {
			templateErrors = append(templateErrors, errors.Wrapf(err, "handle workload template `%s` failed", wd.Name))
			continue
		}
		if tmp.Install != nil {
			tmp.Source = &types.Source{ChartName: tmp.Install.Helm.Name}
			ioStream := util2.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr}
			if err = helm.InstallHelmChart(ioStream, tmp.Install.Helm); err != nil {
				return nil, nil, fmt.Errorf("unable to install helm chart dependency %s(%s from %s) for this workload '%s': %v ", tmp.Install.Helm.Name, tmp.Install.Helm.Version, tmp.Install.Helm.URL, wd.Name, err)
			}
		}
		gvk, err := util.GetGVKFromDefinition(dm, wd.Spec.Reference)
		if err != nil {
			return nil, nil, fmt.Errorf("capability '%s' was not ready: %v ", wd.Name, err)
		}
		tmp.CrdInfo = &types.CRDInfo{
			APIVersion: gvk.GroupVersion().String(),
			Kind:       gvk.Kind,
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
		return nil, nil, fmt.Errorf("list TraitDefinition err: %s", err)
	}

	var templateErrors []error
	for _, td := range traitDefs.Items {
		tmp, err := HandleDefinition(td.Name, syncDir, td.Spec.Reference.Name, td.Annotations, td.Spec.Extension, types.TypeTrait, td.Spec.AppliesToWorkloads)
		if err != nil {
			templateErrors = append(templateErrors, errors.Wrapf(err, "handle trait template `%s` failed\n", td.Name))
			continue
		}
		if tmp.Install != nil {
			tmp.Source = &types.Source{ChartName: tmp.Install.Helm.Name}
			ioStream := util2.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr}
			if err = helm.InstallHelmChart(ioStream, tmp.Install.Helm); err != nil {
				return nil, nil, fmt.Errorf("unable to install helm chart dependency %s(%s from %s) for this trait '%s': %v ", tmp.Install.Helm.Name, tmp.Install.Helm.Version, tmp.Install.Helm.URL, td.Name, err)
			}
		}
		gvk, err := util.GetGVKFromDefinition(dm, td.Spec.Reference)
		if err != nil {
			return nil, nil, fmt.Errorf("capability '%s' was not ready: %v ", td.Name, err)
		}
		tmp.CrdInfo = &types.CRDInfo{
			APIVersion: gvk.GroupVersion().String(),
			Kind:       gvk.Kind,
		}
		templates = append(templates, tmp)
	}
	return templates, templateErrors, nil
}

// HandleDefinition will handle definition to capability
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
		//nolint:errcheck
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
	//nolint:gosec
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

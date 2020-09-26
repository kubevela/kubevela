package plugins

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"path/filepath"

	"github.com/oam-dev/kubevela/api/types"
	cmdutil "github.com/oam-dev/kubevela/pkg/commands/util"
	"github.com/oam-dev/kubevela/pkg/cue"
	"github.com/oam-dev/kubevela/pkg/utils/system"

	corev1alpha2 "github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func GetCapabilitiesFromCluster(ctx context.Context, namespace string, c client.Client, syncDir string, selector labels.Selector) ([]types.Capability, error) {
	workloads, err := GetWorkloadsFromCluster(ctx, namespace, c, syncDir, selector)
	if err != nil {
		return nil, err
	}
	traits, err := GetTraitsFromCluster(ctx, namespace, c, syncDir, selector)
	if err != nil {
		return nil, err
	}
	workloads = append(workloads, traits...)
	return workloads, nil
}

func GetWorkloadsFromCluster(ctx context.Context, namespace string, c client.Client, syncDir string, selector labels.Selector) ([]types.Capability, error) {
	var templates []types.Capability
	var workloadDefs corev1alpha2.WorkloadDefinitionList
	err := c.List(ctx, &workloadDefs, &client.ListOptions{Namespace: namespace, LabelSelector: selector})
	if err != nil {
		return nil, fmt.Errorf("list WorkloadDefinition err: %s", err)
	}

	for _, wd := range workloadDefs.Items {
		tmp, err := HandleDefinition(wd.Name, syncDir, wd.Spec.Reference.Name, wd.Spec.Extension, types.TypeWorkload, nil)
		if err != nil {
			fmt.Printf("[WARN] hanlde workload template `%s` failed with error: %v\n", wd.Name, err)
			continue
		} else {
			fmt.Printf("imported workload `%s`\n", wd.Name)
		}
		if apiVerion, kind := cmdutil.GetAPIVersionKindFromWorkload(wd); apiVerion != "" && kind != "" {
			tmp.CrdInfo = &types.CrdInfo{
				APIVersion: apiVerion,
				Kind:       kind,
			}
		}
		templates = append(templates, tmp)
	}
	return templates, nil
}

func GetTraitsFromCluster(ctx context.Context, namespace string, c client.Client, syncDir string, selector labels.Selector) ([]types.Capability, error) {
	var templates []types.Capability
	var traitDefs corev1alpha2.TraitDefinitionList
	err := c.List(ctx, &traitDefs, &client.ListOptions{Namespace: namespace, LabelSelector: selector})
	if err != nil {
		return nil, fmt.Errorf("list TraitDefinition err: %s", err)
	}

	for _, td := range traitDefs.Items {
		tmp, err := HandleDefinition(td.Name, syncDir, td.Spec.Reference.Name, td.Spec.Extension, types.TypeTrait, td.Spec.AppliesToWorkloads)
		if err != nil {
			fmt.Printf("[WARN] hanlde trait template `%s` failed with error: %v\n", td.Name, err)
			continue
		} else {
			fmt.Printf("imported trait `%s`\n", td.Name)
		}
		if apiVerion, kind := cmdutil.GetAPIVersionKindFromTrait(td); apiVerion != "" && kind != "" {
			tmp.CrdInfo = &types.CrdInfo{
				APIVersion: apiVerion,
				Kind:       kind,
			}
		}
		templates = append(templates, tmp)
	}
	return templates, nil
}

func HandleDefinition(name, syncDir, crdName string, extention *runtime.RawExtension, tp types.CapType, applyTo []string) (types.Capability, error) {
	var tmp types.Capability
	tmp, err := HandleTemplate(extention, name, syncDir)
	if err != nil {
		return types.Capability{}, err
	}
	tmp.Type = tp
	if tp == types.TypeTrait {
		tmp.AppliesTo = applyTo
	}
	tmp.CrdName = crdName
	return tmp, nil
}

func HandleTemplate(in *runtime.RawExtension, name, syncDir string) (types.Capability, error) {
	tmp, err := types.ConvertTemplateJSON2Object(in)
	if err != nil {
		return types.Capability{}, err
	}

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
	tmp.Parameters, tmp.Name, err = cue.GetParameters(filePath)
	if err != nil {
		return types.Capability{}, err
	}
	return tmp, nil
}

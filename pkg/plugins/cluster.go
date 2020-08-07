package plugins

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"path/filepath"

	"k8s.io/apimachinery/pkg/labels"

	"github.com/cloud-native-application/rudrx/pkg/cue"

	"k8s.io/apimachinery/pkg/runtime"

	"github.com/cloud-native-application/rudrx/api/types"

	corev1alpha2 "github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func GetTemplatesFromCluster(ctx context.Context, namespace string, c client.Client, syncDir string, selector labels.Selector) ([]types.Template, error) {
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

func GetWorkloadsFromCluster(ctx context.Context, namespace string, c client.Client, syncDir string, selector labels.Selector) ([]types.Template, error) {
	var templates []types.Template
	var workloadDefs corev1alpha2.WorkloadDefinitionList
	err := c.List(ctx, &workloadDefs, &client.ListOptions{Namespace: namespace, LabelSelector: selector})
	if err != nil {
		return nil, fmt.Errorf("list WorkloadDefinition err: %s", err)
	}

	for _, wd := range workloadDefs.Items {
		var tmp types.Template
		tmp, err := HandleTemplate(wd.Spec.Extension, wd.Name, syncDir)
		if err != nil {
			fmt.Printf("[WARN]handle template %s: %v\n", wd.Name, err)
			continue
		}
		tmp.Type = types.TypeWorkload
		tmp.CrdName = wd.Spec.Reference.Name
		templates = append(templates, tmp)
	}
	return templates, nil
}

func GetTraitsFromCluster(ctx context.Context, namespace string, c client.Client, syncDir string, selector labels.Selector) ([]types.Template, error) {
	var templates []types.Template
	var traitDefs corev1alpha2.TraitDefinitionList
	err := c.List(ctx, &traitDefs, &client.ListOptions{Namespace: namespace, LabelSelector: selector})
	if err != nil {
		return nil, fmt.Errorf("list TraitDefinition err: %s", err)
	}

	for _, td := range traitDefs.Items {
		var tmp types.Template
		tmp, err := HandleTemplate(td.Spec.Extension, td.Name, syncDir)
		if err != nil {
			fmt.Printf("[WARN]handle template %s: %v\n", td.Name, err)
			continue
		}
		tmp.Type = types.TypeTrait
		tmp.AppliesTo = td.Spec.AppliesToWorkloads
		tmp.CrdName = td.Spec.Reference.Name
		templates = append(templates, tmp)
	}
	return templates, nil
}

func HandleTemplate(in *runtime.RawExtension, name, syncDir string) (types.Template, error) {
	tmp, err := types.ConvertTemplateJson2Object(in)
	if err != nil {
		return types.Template{}, err
	}
	if tmp.Template == "" {
		return types.Template{}, errors.New("template not exist in definition")
	}
	filePath := filepath.Join(syncDir, name+".cue")
	err = ioutil.WriteFile(filePath, []byte(tmp.Template), 0644)
	if err != nil {
		return types.Template{}, err
	}
	tmp.DefinitionPath = filePath
	tmp.Parameters, tmp.Name, err = cue.GetParameters(filePath)
	if err != nil {
		return types.Template{}, err
	}
	return tmp, nil
}

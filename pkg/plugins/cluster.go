package plugins

import (
	"context"
	"fmt"

	"github.com/cloud-native-application/rudrx/api/types"

	corev1alpha2 "github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func GetTemplatesFromCluster(ctx context.Context, namespace string, c client.Client) ([]types.Template, error) {
	workloads, err := GetWorkloadsFromCluster(ctx, namespace, c)
	if err != nil {
		return nil, err
	}
	traits, err := GetTraitsFromCluster(ctx, namespace, c)
	if err != nil {
		return nil, err
	}
	workloads = append(workloads, traits...)
	return workloads, nil
}

func GetWorkloadsFromCluster(ctx context.Context, namespace string, c client.Client) ([]types.Template, error) {
	var templates []types.Template
	var workloadDefs corev1alpha2.WorkloadDefinitionList
	err := c.List(ctx, &workloadDefs, &client.ListOptions{Namespace: namespace})
	if err != nil {
		return nil, fmt.Errorf("list WorkloadDefinition err: %s", err)
	}

	for _, wd := range workloadDefs.Items {
		var tmp types.Template
		tmp, err := types.ConvertTemplateJson2Object(wd.Spec.Extension)
		if err != nil {
			fmt.Printf("extract template from workloadDefinition %v err: %v, ignore it\n", wd.Name, err)
			continue
		}
		tmp.Type = types.TypeWorkload
		tmp.Name = wd.Name
		templates = append(templates, tmp)
	}
	return templates, nil
}

func GetTraitsFromCluster(ctx context.Context, namespace string, c client.Client) ([]types.Template, error) {
	var templates []types.Template
	var traitDefs corev1alpha2.TraitDefinitionList
	err := c.List(ctx, &traitDefs, &client.ListOptions{Namespace: namespace})
	if err != nil {
		return nil, fmt.Errorf("list TraitDefinition err: %s", err)
	}

	for _, td := range traitDefs.Items {
		var tmp types.Template
		tmp, err := types.ConvertTemplateJson2Object(td.Spec.Extension)
		if err != nil {
			fmt.Printf("extract template from workloadDefinition %v err: %v, ignore it\n", td.Name, err)
			continue
		}
		tmp.Type = types.TypeTrait
		tmp.Name = td.Name
		templates = append(templates, tmp)
	}
	return templates, nil
}

package util

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/cloud-native-application/rudrx/api/types"

	corev1alpha2 "github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"context"
	"encoding/json"

	"k8s.io/klog"
)

const (
	DefaultErrorExitCode = 1
)

func Print(msg string) {
	if klog.V(2) {
		klog.FatalDepth(2, msg)
	}
	if len(msg) > 0 {
		// add newline if needed
		if !strings.HasSuffix(msg, "\n") {
			msg += "\n"
		}
		fmt.Fprint(os.Stderr, msg)
	}
}

func fatal(msg string, code int) {
	if klog.V(2) {
		klog.FatalDepth(2, msg)
	}
	if len(msg) > 0 {
		// add newline if needed
		if !strings.HasSuffix(msg, "\n") {
			msg += "\n"
		}
		fmt.Fprint(os.Stderr, msg)
	}
	os.Exit(code)
}

func CheckErr(err error) {
	if err == nil {
		return
	}
	msg := err.Error()
	if !strings.HasPrefix(msg, "error: ") {
		msg = fmt.Sprintf("error: %s", msg)
	}
	fatal(msg, DefaultErrorExitCode)
}

func GetComponent(ctx context.Context, c client.Client, componentName string, namespace string) (corev1alpha2.Component, error) {
	var component corev1alpha2.Component
	err := c.Get(ctx, client.ObjectKey{Name: componentName, Namespace: namespace}, &component)
	return component, err
}

func ListTraitDefinitionsByApplicationConfiguration(app corev1alpha2.ApplicationConfiguration) []corev1alpha2.TraitDefinition {
	var traitDefinitionList []corev1alpha2.TraitDefinition
	for _, t := range app.Spec.Components[0].Traits {
		var trait corev1alpha2.TraitDefinition
		if err := json.Unmarshal(t.Trait.Raw, &trait); err == nil {
			traitDefinitionList = append(traitDefinitionList, trait)
		}
	}
	return traitDefinitionList
}

func GetTraitDefinitionByName(ctx context.Context, c client.Client, namespace string, traitName string) (corev1alpha2.TraitDefinition, error) {
	var t corev1alpha2.TraitDefinition
	err := c.Get(ctx, client.ObjectKey{Name: traitName, Namespace: namespace}, &t)
	return t, err
}

func GetTraitDefinitionByAlias(ctx context.Context, c client.Client, traitAlias string) (corev1alpha2.TraitDefinition, error) {
	var traitDefinitionList corev1alpha2.TraitDefinitionList
	var traitDefinition corev1alpha2.TraitDefinition
	err := c.List(ctx, &traitDefinitionList)
	if err == nil {
		for _, t := range traitDefinitionList.Items {
			template, err := types.ConvertTemplateJson2Object(t.Spec.Extension)
			if err == nil && strings.EqualFold(template.Name, traitAlias) {
				traitDefinition = t
				break
			}
		}
	}
	return traitDefinition, err
}

// GetTraitNameAndAlias return the name and alias of a TraitDefinition by a string which might be
// the trait name, the trait alias, or invalid name
func GetTraitApiVersionKind(ctx context.Context, c client.Client, namespace string, name string) (string, string, error) {

	t, err := GetTraitDefinitionByName(ctx, c, namespace, name)
	if err != nil {
		return "", "", err
	}
	apiVersion := t.Annotations[types.AnnApiVersion]
	kind := t.Annotations[types.AnnKind]
	return apiVersion, kind, nil
}

func GetApiVersionKindFromTrait(td corev1alpha2.TraitDefinition) (string, string) {
	return td.Annotations[types.AnnApiVersion], td.Annotations[types.AnnKind]
}

func GetApiVersionKindFromWorkload(td corev1alpha2.WorkloadDefinition) (string, string) {
	return td.Annotations[types.AnnApiVersion], td.Annotations[types.AnnKind]
}

func GetWorkloadNameAliasKind(ctx context.Context, c client.Client, namespace string, workloadName string) (string, string, string) {
	var name, alias, kind string

	w, err := GetWorkloadDefinitionByName(ctx, c, namespace, workloadName)

	if err == nil { // workloadName is complete name
		var workloadTemplate types.Capability
		workloadTemplate, err := types.ConvertTemplateJson2Object(w.Spec.Extension)
		if err == nil {
			name, alias = w.Name, workloadTemplate.Name
		}
	} else { // workloadName is alias or kind
		w, err := GetWorkloadDefinitionByAlias(ctx, c, name)
		if err == nil {
			workloadTemplate, err := types.ConvertTemplateJson2Object(w.Spec.Extension)
			if err == nil {
				name, alias, kind = w.Name, workloadTemplate.Name, w.Kind
			}

		}
	}

	return name, alias, kind
}

func GetWorkloadDefinitionByName(ctx context.Context, c client.Client, namespace string, name string) (corev1alpha2.WorkloadDefinition, error) {
	var w corev1alpha2.WorkloadDefinition
	err := c.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, &w)
	return w, err
}

func GetWorkloadDefinitionByAlias(ctx context.Context, c client.Client, traitAlias string) (corev1alpha2.WorkloadDefinition, error) {
	var workloadDefinitionList corev1alpha2.WorkloadDefinitionList
	var workloadDefinition corev1alpha2.WorkloadDefinition
	if err := c.List(ctx, &workloadDefinitionList); err == nil {
		for _, t := range workloadDefinitionList.Items {
			if strings.EqualFold(t.ObjectMeta.Annotations["short"], traitAlias) {
				workloadDefinition = t
				break
			}
		}
	}

	return workloadDefinition, nil
}

func PrintUsageIntroduce(cmd *cobra.Command, introduce string) {
	cmd.Println(introduce)
	cmd.Println()
}

func PrintUsage(cmd *cobra.Command, subcmds []*cobra.Command) {
	printUsage := func(cmd *cobra.Command) {
		useline := cmd.UseLine()
		if !strings.HasPrefix(useline, "vela ") {
			useline = "vela " + useline
		}
		cmd.Printf("  %s\t\t%s\n", useline, cmd.Long)
	}
	cmd.Println("Usage:")
	for _, sub := range subcmds {
		printUsage(sub)
	}
	cmd.Println()
}
func PrintExample(cmd *cobra.Command, subcmds []*cobra.Command) {
	printExample := func(cmd *cobra.Command) {
		cmd.Printf("  %s\n", cmd.Example)
	}
	cmd.Println("Examples:")
	for _, sub := range subcmds {
		printExample(sub)
	}
	cmd.Println()
}

func PrintFlags(cmd *cobra.Command, subcmds []*cobra.Command) {
	cmd.Println("Flags:")
	for _, sub := range subcmds {
		if sub.HasLocalFlags() {
			fmt.Printf(sub.LocalFlags().FlagUsages())
		}
	}
	cmd.Println()
}

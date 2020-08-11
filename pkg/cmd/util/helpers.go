package util

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/spf13/cobra"

	"github.com/cloud-native-application/rudrx/api/types"
	"github.com/cloud-native-application/rudrx/pkg/cue"
	"github.com/cloud-native-application/rudrx/pkg/utils/system"

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

type ApplicationMeta struct {
	Name        string   `json:"name"`
	Workload    string   `json:"workload,omitempty"`
	Traits      []string `json:"traits,omitempty"`
	Status      string   `json:"status,omitempty"`
	CreatedTime string   `json:"created,omitempty"`
}

func GetComponent(ctx context.Context, c client.Client, componentName string, namespace string) (corev1alpha2.Component, error) {
	var component corev1alpha2.Component
	err := c.Get(ctx, client.ObjectKey{Name: componentName, Namespace: namespace}, &component)
	return component, err
}

func GetTraitAliasByComponentTraitList(ctx context.Context, c client.Client, componentTraitList []corev1alpha2.ComponentTrait) []string {
	var traitAlias []string
	for _, t := range componentTraitList {
		_, _, kind := GetGVKFromRawExtension(t.Trait)
		alias := GetTraitAliasByKind(ctx, c, kind)
		traitAlias = append(traitAlias, alias)
	}
	return traitAlias
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

/*
	Get application list by optional filter `applicationName`
	Application name is equal to Component name as currently vela only supports one component exists in one application
*/
func RetrieveApplicationsByName(ctx context.Context, c client.Client, applicationName string, namespace string) ([]ApplicationMeta, error) {
	var applicationMetaList []ApplicationMeta
	var applicationList corev1alpha2.ApplicationConfigurationList

	if applicationName != "" {
		var application corev1alpha2.ApplicationConfiguration
		err := c.Get(ctx, client.ObjectKey{Name: applicationName, Namespace: namespace}, &application)

		if err != nil {
			return applicationMetaList, err
		}

		applicationList.Items = append(applicationList.Items, application)
	} else {
		err := c.List(ctx, &applicationList, &client.ListOptions{Namespace: namespace})
		if err != nil {
			return applicationMetaList, err
		}
	}

	for _, a := range applicationList.Items {
		for _, com := range a.Spec.Components {
			componentName := com.ComponentName
			component, err := GetComponent(ctx, c, componentName, namespace)
			if err != nil {
				return applicationMetaList, err
			}
			var workload corev1alpha2.WorkloadDefinition
			if err := json.Unmarshal(component.Spec.Workload.Raw, &workload); err == nil {
				workloadName := workload.TypeMeta.Kind
				traitAlias := GetTraitAliasByComponentTraitList(ctx, c, com.Traits)
				var status = "UNKNOWN"
				if len(a.Status.Conditions) != 0 {
					status = string(a.Status.Conditions[0].Status)
				}
				applicationMetaList = append(applicationMetaList, ApplicationMeta{
					Name:        a.Name,
					Workload:    workloadName,
					Traits:      traitAlias,
					Status:      status,
					CreatedTime: a.ObjectMeta.CreationTimestamp.String(),
				})
			}
		}
	}
	return applicationMetaList, nil
}

func GetTraitAliasByTraitDefinition(traitDefinition corev1alpha2.TraitDefinition) (string, error) {
	velaApplicationFolder := filepath.Join("~/.vela", "applications")
	system.StatAndCreate(velaApplicationFolder)
	d, _ := ioutil.TempDir(velaApplicationFolder, "cue")
	defer os.RemoveAll(d)
	template, err := HandleTemplate(traitDefinition.Spec.Extension, traitDefinition.Name, d)
	if err != nil {
		return "", nil
	}
	return template.Name, nil
}

func GetTraitDefinitionByName(ctx context.Context, c client.Client, namespace string, traitName string) (corev1alpha2.TraitDefinition, error) {
	var t corev1alpha2.TraitDefinition
	err := c.Get(ctx, client.ObjectKey{Name: traitName, Namespace: namespace}, &t)
	return t, err
}

func GetTraitAliasByKind(ctx context.Context, c client.Client, traitKind string) string {
	var traitAlias string
	t, err := GetTraitDefinitionByKind(ctx, c, traitKind)
	if err != nil {
		return traitKind
	}

	if traitAlias, err = GetTraitAliasByTraitDefinition(t); err != nil {
		return traitKind
	}

	return traitAlias
}

func GetTraitDefinitionByKind(ctx context.Context, c client.Client, traitKind string) (corev1alpha2.TraitDefinition, error) {
	var traitDefinitionList corev1alpha2.TraitDefinitionList
	var traitDefinition corev1alpha2.TraitDefinition
	if err := c.List(ctx, &traitDefinitionList); err != nil {
		return traitDefinition, err
	}
	for _, t := range traitDefinitionList.Items {
		if t.Annotations["oam.appengine.info/kind"] == traitKind {
			return t, nil
		}
	}
	return traitDefinition, errors.New(fmt.Sprintf("Could not find TraitDefinition by kind %s", traitKind))
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
	apiVersion := t.Annotations["oam.appengine.info/apiVersion"]
	kind := t.Annotations["oam.appengine.info/kind"]
	return apiVersion, kind, nil
}

func GetWorkloadNameAliasKind(ctx context.Context, c client.Client, namespace string, workloadName string) (string, string, string) {
	var name, alias, kind string

	w, err := GetWorkloadDefinitionByName(ctx, c, namespace, workloadName)

	if err == nil { // workloadName is complete name
		var workloadTemplate types.Template
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

func GetGVKFromRawExtension(extension runtime.RawExtension) (string, string, string) {
	if extension.Object != nil {
		gvk := extension.Object.GetObjectKind().GroupVersionKind()
		return gvk.Group, gvk.Version, gvk.Kind
	}
	var data map[string]interface{}
	// leverage Admission Controller to do the check
	_ = json.Unmarshal(extension.Raw, &data)
	obj := unstructured.Unstructured{Object: data}
	gvk := obj.GroupVersionKind()
	return gvk.Group, gvk.Version, gvk.Kind
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

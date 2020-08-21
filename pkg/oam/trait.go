package oam

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/cloud-native-application/rudrx/pkg/server/apis"

	"github.com/cloud-native-application/rudrx/api/types"
	"github.com/cloud-native-application/rudrx/pkg/plugins"
	"github.com/cloud-native-application/rudrx/pkg/utils/system"
	corev1alpha2 "github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"

	plur "github.com/gertd/go-pluralize"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

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

func GetTraitAliasByComponentTraitList(ctx context.Context, c client.Client, componentTraitList []corev1alpha2.ComponentTrait) []string {
	var traitAlias []string
	for _, t := range componentTraitList {
		_, _, kind := GetGVKFromRawExtension(t.Trait)
		alias := GetTraitAliasByKind(ctx, c, kind)
		traitAlias = append(traitAlias, alias)
	}
	return traitAlias
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
func GetTraitAliasByTraitDefinition(traitDefinition corev1alpha2.TraitDefinition) (string, error) {
	velaApplicationFolder := filepath.Join("~/.vela", "applications")
	if _, err := system.CreateIfNotExist(velaApplicationFolder); err != nil {
		return "", nil
	}

	d, _ := ioutil.TempDir(velaApplicationFolder, "cue")
	defer os.RemoveAll(d)
	template, err := plugins.HandleTemplate(traitDefinition.Spec.Extension, traitDefinition.Name, d)
	if err != nil {
		return "", nil
	}
	return template.Name, nil
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
	return traitDefinition, fmt.Errorf("could not find TraitDefinition by kind %s", traitKind)
}

func ListTraitDefinitions(workloadName *string) ([]types.Capability, error) {
	var traitList []types.Capability
	traits, err := plugins.LoadInstalledCapabilityWithType(types.TypeTrait)
	if err != nil {
		return traitList, err
	}
	workloads, err := plugins.LoadInstalledCapabilityWithType(types.TypeWorkload)
	if err != nil {
		return traitList, err
	}
	traitList = assembleDefinitionList(traits, workloads, workloadName)
	return traitList, nil
}

func GetTraitDefinition(workloadName *string, capabilityAlias string) (types.Capability, error) {
	var traitDef types.Capability
	traitCap, err := plugins.GetInstalledCapabilityWithCapAlias(types.TypeTrait, capabilityAlias)
	if err != nil {
		return traitDef, err
	}
	workloadsCap, err := plugins.LoadInstalledCapabilityWithType(types.TypeWorkload)
	if err != nil {
		return traitDef, err
	}
	traitList := assembleDefinitionList([]types.Capability{traitCap}, workloadsCap, workloadName)
	if len(traitList) != 1 {
		return traitDef, fmt.Errorf("could not get installed capability by %s", capabilityAlias)
	}
	traitDef = traitList[0]
	return traitDef, nil
}

func assembleDefinitionList(traits []types.Capability, workloads []types.Capability, workloadName *string) []types.Capability {
	var traitList []types.Capability
	for _, t := range traits {
		convertedApplyTo := ConvertApplyTo(t.AppliesTo, workloads)
		if *workloadName != "" {
			if !In(convertedApplyTo, *workloadName) {
				continue
			}
			convertedApplyTo = []string{*workloadName}
		}
		t.AppliesTo = convertedApplyTo
		traitList = append(traitList, t)
	}
	return traitList
}

func ConvertApplyTo(applyTo []string, workloads []types.Capability) []string {
	var converted []string
	for _, v := range applyTo {
		newName, exist := check(v, workloads)
		if !exist {
			continue
		}
		converted = append(converted, newName)
	}
	return converted
}

func check(applyto string, workloads []types.Capability) (string, bool) {
	for _, v := range workloads {
		if Parse(applyto) == v.CrdName {
			return v.Name, true
		}
	}
	return "", false
}

func In(l []string, v string) bool {
	for _, ll := range l {
		if ll == v {
			return true
		}
	}
	return false
}

func Parse(applyTo string) string {
	l := strings.Split(applyTo, "/")
	if len(l) != 2 {
		return applyTo
	}
	apigroup, versionKind := l[0], l[1]
	l = strings.Split(versionKind, ".")
	if len(l) != 2 {
		return applyTo
	}
	return plur.NewClient().Plural(strings.ToLower(l[1])) + "." + apigroup
}

func SimplifyCapabilityStruct(capabilityList []types.Capability) []apis.TraitMeta {
	var traitList []apis.TraitMeta
	for _, c := range capabilityList {
		traitList = append(traitList, apis.TraitMeta{
			Name:       c.Name,
			Definition: c.CrdName,
			AppliesTo:  c.AppliesTo,
		})
	}
	return traitList
}

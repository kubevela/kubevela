package oam

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/cloud-native-application/rudrx/pkg/plugins"
	"github.com/cloud-native-application/rudrx/pkg/utils/system"
	corev1alpha2 "github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"

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
	return traitDefinition, errors.New(fmt.Sprintf("Could not find TraitDefinition by kind %s", traitKind))
}

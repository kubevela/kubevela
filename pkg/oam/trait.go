package oam

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"cuelang.org/go/cue"
	"github.com/cloud-native-application/rudrx/pkg/application"
	"github.com/spf13/pflag"

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

//AddOrUpdateTrait attach trait to workload
func AddOrUpdateTrait(envName string, appName string, workloadName string, flagSet *pflag.FlagSet, template types.Capability) (*application.Application, error) {
	if appName == "" {
		appName = workloadName
	}
	app, err := application.Load(envName, appName)
	if err != nil {
		return app, err
	}
	traitAlias := template.Name
	traitData, err := app.GetTraitsByType(workloadName, traitAlias)
	if err != nil {
		return app, err
	}
	for _, v := range template.Parameters {
		flagValue, _ := flagSet.GetString(v.Name)
		switch v.Type {
		case cue.IntKind:
			d, _ := strconv.ParseInt(flagValue, 10, 64)
			traitData[v.Name] = d
		case cue.StringKind:
			traitData[v.Name] = flagValue
		case cue.BoolKind:
			d, _ := strconv.ParseBool(flagValue)
			traitData[v.Name] = d
		case cue.NumberKind, cue.FloatKind:
			d, _ := strconv.ParseFloat(flagValue, 64)
			traitData[v.Name] = d
		}
	}
	if err = app.SetTrait(workloadName, traitAlias, traitData); err != nil {
		return app, err
	}
	return app, app.Save(envName)
}

func AttachTrait(c *gin.Context, body apis.TraitBody) (string, error) {
	// Prepare
	var appObj *application.Application
	fs := pflag.NewFlagSet("trait", pflag.ContinueOnError)
	for _, f := range body.Flags {
		fs.String(f.Name, f.Value, "")
	}
	var staging = false
	var err error
	if body.Staging != "" {
		staging, err = strconv.ParseBool(body.Staging)
		if err != nil {
			return "", err
		}
	}
	traitAlias := body.Name
	template, err := plugins.GetInstalledCapabilityWithCapAlias(types.TypeTrait, traitAlias)
	if err != nil {
		return "", err
	}
	appObj, err = AddOrUpdateTrait(body.EnvName, body.AppGroup, body.WorkloadName, fs, template)
	if err != nil {
		return "", err
	}
	// Run step
	env, err := GetEnvByName(body.EnvName)
	if err != nil {
		return "", err
	}
	kubeClient := c.MustGet("KubeClient")
	return TraitOperationRun(c, kubeClient.(client.Client), env, appObj, staging)
}

func TraitOperationRun(ctx context.Context, c client.Client, env *types.EnvMeta, appObj *application.Application, staging bool) (string, error) {
	if staging {
		return "Staging saved", nil
	}
	err := appObj.Run(ctx, c, env)
	if err != nil {
		return "", err
	}
	return "Succeeded!", nil
}

func PrepareDetachTrait(envName string, traitType string, workloadName string, appName string) (*application.Application, error) {
	var appObj *application.Application
	var err error
	if appName == "" {
		appName = workloadName
	}
	if appObj, err = application.Load(envName, appName); err != nil {
		return appObj, err
	}

	if err = appObj.RemoveTrait(workloadName, traitType); err != nil {
		return appObj, err
	}
	return appObj, appObj.Save(envName)
}

func DetachTrait(c *gin.Context, envName string, traitType string, workloadName string, appName string, staging bool) (string, error) {
	var appObj *application.Application
	var err error
	if appName == "" {
		appName = workloadName
	}
	if appObj, err = PrepareDetachTrait(envName, traitType, workloadName, appName); err != nil {
		return "", err
	}
	// Run
	env, err := GetEnvByName(envName)
	if err != nil {
		return "", err
	}
	kubeClient := c.MustGet("KubeClient")
	return TraitOperationRun(c, kubeClient.(client.Client), env, appObj, staging)
}

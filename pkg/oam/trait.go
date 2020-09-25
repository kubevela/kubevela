package oam

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"cuelang.org/go/cue"
	corev1alpha2 "github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
	plur "github.com/gertd/go-pluralize"
	"github.com/gin-gonic/gin"
	"github.com/spf13/pflag"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/api/types"
	"github.com/oam-dev/kubevela/pkg/application"
	"github.com/oam-dev/kubevela/pkg/plugins"
	"github.com/oam-dev/kubevela/pkg/server/apis"
)

func GetTraitDefNameFromRaw(extension runtime.RawExtension) string {
	if extension.Raw == nil {
		extension.Raw, _ = extension.MarshalJSON()
	}
	var data map[string]interface{}
	// leverage Admission Controller to do the check
	_ = json.Unmarshal(extension.Raw, &data)
	obj := unstructured.Unstructured{Object: data}
	ann := obj.GetAnnotations()
	if ann == nil {
		return obj.GetKind()
	}
	return ann[types.AnnTraitDef]
}

func GetTraitAliasByComponentTraitList(componentTraitList []corev1alpha2.ComponentTrait) []string {
	var traitAlias []string
	for _, t := range componentTraitList {
		traitAlias = append(traitAlias, GetTraitDefNameFromRaw(t.Trait))
	}
	return traitAlias
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

func ValidateAndMutateForCore(traitType, workloadName string, flags *pflag.FlagSet, env *types.EnvMeta) error {
	switch traitType {
	case "route":
		domain, _ := flags.GetString("domain")
		if domain == "" {
			if env.Domain == "" {
				return fmt.Errorf("--domain is required if not set in env")
			}
			if strings.HasPrefix(env.Domain, "https://") {
				env.Domain = strings.TrimPrefix(env.Domain, "https://")
			}
			if strings.HasPrefix(env.Domain, "http://") {
				env.Domain = strings.TrimPrefix(env.Domain, "http://")
			}
			if err := flags.Set("domain", workloadName+"."+env.Domain); err != nil {
				return fmt.Errorf("set flag for vela-core trait('route') err %v, please make sure your template is right", err)
			}
		}
		issuer, _ := flags.GetString("issuer")
		if issuer == "" {
			if env.Issuer == "" {
				return fmt.Errorf("--issuer is required, you can also set email in env and let it generate automatically")
			}
			if err := flags.Set("issuer", env.Issuer); err != nil {
				return fmt.Errorf("set flag for vela-core trait('route') err %v, please make sure your template is right", err)
			}
		}
	}
	return nil
}

//AddOrUpdateTrait attach trait to workload
func AddOrUpdateTrait(env *types.EnvMeta, appName string, workloadName string, flagSet *pflag.FlagSet, template types.Capability) (*application.Application, error) {
	err := ValidateAndMutateForCore(template.Name, workloadName, flagSet, env)
	if err != nil {
		return nil, err
	}
	if appName == "" {
		appName = workloadName
	}
	app, err := application.Load(env.Name, appName)
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
	return app, app.Save(env.Name)
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
	// Run step
	env, err := GetEnvByName(body.EnvName)
	if err != nil {
		return "", err
	}

	appObj, err = AddOrUpdateTrait(env, body.AppName, body.WorkloadName, fs, template)
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

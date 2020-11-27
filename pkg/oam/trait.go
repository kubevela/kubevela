package oam

import (
	"context"
	"fmt"
	"strings"

	"cuelang.org/go/cue"
	plur "github.com/gertd/go-pluralize"
	"github.com/spf13/pflag"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/application"
	cmdutil "github.com/oam-dev/kubevela/pkg/commands/util"
	"github.com/oam-dev/kubevela/pkg/plugins"
)

// ListTraitDefinitions will list all definition include traits and workloads
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
	traitList = convertAllAppliyToList(traits, workloads, workloadName)
	return traitList, nil
}

// GetTraitDefinition will get trait capability with applyTo converted
func GetTraitDefinition(workloadName *string, traitType string) (types.Capability, error) {
	var traitDef types.Capability
	traitCap, err := plugins.GetInstalledCapabilityWithCapName(types.TypeTrait, traitType)
	if err != nil {
		return traitDef, err
	}
	workloadsCap, err := plugins.LoadInstalledCapabilityWithType(types.TypeWorkload)
	if err != nil {
		return traitDef, err
	}
	traitList := convertAllAppliyToList([]types.Capability{traitCap}, workloadsCap, workloadName)
	if len(traitList) != 1 {
		return traitDef, fmt.Errorf("could not get installed capability by %s", traitType)
	}
	traitDef = traitList[0]
	return traitDef, nil
}

func convertAllAppliyToList(traits []types.Capability, workloads []types.Capability, workloadName *string) []types.Capability {
	var traitList []types.Capability
	for _, t := range traits {
		convertedApplyTo := ConvertApplyTo(t.AppliesTo, workloads)
		if *workloadName != "" {
			if !in(convertedApplyTo, *workloadName) {
				continue
			}
			convertedApplyTo = []string{*workloadName}
		}
		t.AppliesTo = convertedApplyTo
		traitList = append(traitList, t)
	}
	return traitList
}

// ConvertApplyTo will convert applyTo slice to workload capability name if CRD matches
func ConvertApplyTo(applyTo []string, workloads []types.Capability) []string {
	var converted []string
	for _, v := range applyTo {
		newName, exist := check(v, workloads)
		if !exist {
			continue
		}
		if !in(converted, newName) {
			converted = append(converted, newName)
		}
	}
	return converted
}

func check(applyto string, workloads []types.Capability) (string, bool) {
	for _, v := range workloads {
		if Parse(applyto) == v.CrdName || Parse(applyto) == v.Name {
			return v.Name, true
		}
	}
	return "", false
}

func in(l []string, v string) bool {
	for _, ll := range l {
		if ll == v {
			return true
		}
	}
	return false
}

// Parse will parse applyTo(with format apigroup/Version.Kind) to crd name by just calculate the plural of kind word.
// TODO we should use discoverymapper instead of calculate plural
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

// ValidateAndMutateForCore was built in validate and mutate function for core workloads and traits
func ValidateAndMutateForCore(traitType, workloadName string, flags *pflag.FlagSet, env *types.EnvMeta) error {
	switch traitType {
	case "route":
		domain, _ := flags.GetString("domain")
		if domain == "" {
			if env.Domain == "" {
				return fmt.Errorf("--domain is required if not contain in environment")
			}
			if strings.HasPrefix(env.Domain, "https://") {
				env.Domain = strings.TrimPrefix(env.Domain, "https://")
			}
			if strings.HasPrefix(env.Domain, "http://") {
				env.Domain = strings.TrimPrefix(env.Domain, "http://")
			}
			if err := flags.Set("domain", workloadName+"."+env.Domain); err != nil {
				return fmt.Errorf("set flag for vela-core trait('route') err %w, please make sure your template is right", err)
			}
		}
		issuer, _ := flags.GetString("issuer")
		if issuer == "" && env.Issuer != "" {
			if err := flags.Set("issuer", env.Issuer); err != nil {
				return fmt.Errorf("set flag for vela-core trait('route') err %w, please make sure your template is right", err)
			}
		}
	default:
		// extend other trait here in the future
	}
	return nil
}

// AddOrUpdateTrait attach trait to workload
func AddOrUpdateTrait(env *types.EnvMeta, appName string, componentName string, flagSet *pflag.FlagSet, template types.Capability) (*application.Application, error) {
	err := ValidateAndMutateForCore(template.Name, componentName, flagSet, env)
	if err != nil {
		return nil, err
	}
	if appName == "" {
		appName = componentName
	}
	app, err := application.Load(env.Name, appName)
	if err != nil {
		return app, err
	}
	traitAlias := template.Name
	traitData, err := app.GetTraitsByType(componentName, traitAlias)
	if err != nil {
		return app, err
	}
	for _, v := range template.Parameters {
		name := v.Name
		if v.Alias != "" {
			name = v.Alias
		}
		// nolint:exhaustive
		switch v.Type {
		case cue.IntKind:
			traitData[v.Name], err = flagSet.GetInt64(name)
		case cue.StringKind:
			traitData[v.Name], err = flagSet.GetString(name)
		case cue.BoolKind:
			traitData[v.Name], err = flagSet.GetBool(name)
		case cue.NumberKind, cue.FloatKind:
			traitData[v.Name], err = flagSet.GetFloat64(name)
		default:
			// Currently we don't support get value from complex type
			continue
		}

		if err != nil {
			return nil, fmt.Errorf("get flag(s) \"%s\" err %w", name, err)
		}
	}
	if err = app.SetTrait(componentName, traitAlias, traitData); err != nil {
		return app, err
	}
	return app, app.Save(env.Name)
}

// TraitOperationRun will check if it's a stage operation before run
func TraitOperationRun(ctx context.Context, c client.Client, env *types.EnvMeta, appObj *application.Application,
	staging bool, io cmdutil.IOStreams) (string, error) {
	if staging {
		return "Staging saved", nil
	}
	err := appObj.BuildRun(ctx, c, env, io)
	if err != nil {
		return "", err
	}
	return "Deployed!", nil
}

// PrepareDetachTrait will detach trait in local AppFile
func PrepareDetachTrait(envName string, traitType string, componentName string, appName string) (*application.Application, error) {
	var appObj *application.Application
	var err error
	if appName == "" {
		appName = componentName
	}
	if appObj, err = application.Load(envName, appName); err != nil {
		return appObj, err
	}

	if err = appObj.RemoveTrait(componentName, traitType); err != nil {
		return appObj, err
	}
	return appObj, appObj.Save(envName)
}

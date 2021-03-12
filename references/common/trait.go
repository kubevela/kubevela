package common

import (
	"context"
	"fmt"
	"strings"

	plur "github.com/gertd/go-pluralize"
	client2 "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/references/plugins"
)

// ListTraitDefinitions will list all definition include traits and workloads
func ListTraitDefinitions(userNamespace string, c types.Args, workloadName *string) ([]types.Capability, error) {
	var traitList []types.Capability
	traits, err := plugins.LoadInstalledCapabilityWithType(userNamespace, c, types.TypeTrait)
	if err != nil {
		return traitList, err
	}
	workloads, err := plugins.LoadInstalledCapabilityWithType(userNamespace, c, types.TypeWorkload)
	if err != nil {
		return traitList, err
	}
	traitList = convertAllApplyToList(traits, workloads, workloadName)
	return traitList, nil
}

// ListRawTraitDefinitions will list raw definition
func ListRawTraitDefinitions(userNamespace string, c types.Args) ([]v1alpha2.TraitDefinition, error) {
	client, err := c.GetClient()
	if err != nil {
		return nil, err
	}
	ctx := util.SetNamespaceInCtx(context.Background(), userNamespace)
	traitList := v1alpha2.TraitDefinitionList{}
	if err = client.List(ctx, &traitList, client2.InNamespace(userNamespace)); err != nil {
		return nil, err
	}
	sysTraitList := v1alpha2.TraitDefinitionList{}
	if err = client.List(ctx, &sysTraitList, client2.InNamespace(oam.SystemDefinitonNamespace)); err != nil {
		return nil, err
	}
	return append(traitList.Items, sysTraitList.Items...), nil
}

// ListRawWorkloadDefinitions will list raw definition
func ListRawWorkloadDefinitions(userNamespace string, c types.Args) ([]v1alpha2.WorkloadDefinition, error) {
	client, err := c.GetClient()
	if err != nil {
		return nil, err
	}
	ctx := util.SetNamespaceInCtx(context.Background(), userNamespace)
	workloadList := v1alpha2.WorkloadDefinitionList{}
	if err = client.List(ctx, &workloadList); err != nil {
		return nil, err
	}
	sysWorkloadList := v1alpha2.WorkloadDefinitionList{}
	if err = client.List(ctx, &sysWorkloadList, client2.InNamespace(oam.SystemDefinitonNamespace)); err != nil {
		return nil, err
	}
	return append(workloadList.Items, sysWorkloadList.Items...), nil
}

// GetTraitDefinition will get trait capability with applyTo converted
func GetTraitDefinition(userNamespace string, c types.Args, workloadName *string, traitType string) (types.Capability, error) {
	var traitDef types.Capability
	traitCap, err := plugins.GetInstalledCapabilityWithCapName(types.TypeTrait, traitType)
	if err != nil {
		return traitDef, err
	}
	workloadsCap, err := plugins.LoadInstalledCapabilityWithType(userNamespace, c, types.TypeWorkload)
	if err != nil {
		return traitDef, err
	}
	traitList := convertAllApplyToList([]types.Capability{traitCap}, workloadsCap, workloadName)
	if len(traitList) != 1 {
		return traitDef, fmt.Errorf("could not get installed capability by %s", traitType)
	}
	traitDef = traitList[0]
	return traitDef, nil
}

func convertAllApplyToList(traits []types.Capability, workloads []types.Capability, workloadName *string) []types.Capability {
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
	if in(applyTo, "*") {
		converted = append(converted, "*")
	} else {
		for _, v := range applyTo {
			newName, exist := check(v, workloads)
			if !exist {
				continue
			}
			if !in(converted, newName) {
				converted = append(converted, newName)
			}
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

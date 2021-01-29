package applicationdeployment

import (
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
)

// FindCommonComponent finds the common components in both the source and target application
// the source can be nil
func FindCommonComponent(targetApp, sourceApp *v1alpha2.Application) []string {
	var commonComponents []string
	if sourceApp == nil {
		for _, comp := range targetApp.Spec.Components {
			commonComponents = append(commonComponents, comp.WorkloadType)
		}
		return commonComponents
	}
	// find the common components in both the source and target application
	// write an O(N) algorithm just for fun, totally doesn't worth the extra space
	targetComponents := make(map[string]bool)
	for _, comp := range targetApp.Spec.Components {
		targetComponents[comp.WorkloadType] = true
	}
	for _, comp := range sourceApp.Spec.Components {
		if targetComponents[comp.WorkloadType] {
			commonComponents = append(commonComponents, comp.WorkloadType)
		}
	}
	return commonComponents
}

package applicationrollout

import (
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/pkg/controller/utils"
)

// FindCommonComponent finds the common components in both the source and target application
// the source can be nil
func FindCommonComponent(targetApp, sourceApp *v1alpha2.ApplicationConfiguration) []string {
	var commonComponents []string
	if sourceApp == nil {
		for _, comp := range targetApp.Spec.Components {
			commonComponents = append(commonComponents, utils.ExtractComponentName(comp.RevisionName))
		}
		return commonComponents
	}
	// find the common components in both the source and target application
	// write an O(N) algorithm just for fun, totally doesn't worth the extra space
	targetComponents := make(map[string]bool)
	for _, comp := range targetApp.Spec.Components {
		targetComponents[utils.ExtractComponentName(comp.RevisionName)] = true
	}
	for _, comp := range sourceApp.Spec.Components {
		revisionName := utils.ExtractComponentName(comp.RevisionName)
		if targetComponents[revisionName] {
			commonComponents = append(commonComponents, revisionName)
		}
	}
	return commonComponents
}

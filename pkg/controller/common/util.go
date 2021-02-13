package common

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	"github.com/crossplane/crossplane-runtime/pkg/logging"
	v1 "k8s.io/api/apps/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

// ConstructRevisionName will generate revisionName from componentName
// will be <componentName>-v<RevisionNumber>, for example: comp-v1
func ConstructRevisionName(componentName string, revision int64) string {
	return strings.Join([]string{componentName, fmt.Sprintf("v%d", revision)}, "-")
}

// ExtractComponentName will extract componentName from revisionName
func ExtractComponentName(revisionName string) string {
	splits := strings.Split(revisionName, "-")
	return strings.Join(splits[0:len(splits)-1], "-")
}

// CompareWithRevision compares a component's spec with the component's latest revision content
func CompareWithRevision(ctx context.Context, c client.Client, logger logging.Logger, componentName, nameSpace,
	latestRevision string, curCompSpec *v1alpha2.ComponentSpec) (bool, error) {
	oldRev := &v1.ControllerRevision{}
	if err := c.Get(ctx, client.ObjectKey{Namespace: nameSpace, Name: latestRevision}, oldRev); err != nil {
		logger.Info(fmt.Sprintf("get old controllerRevision %s error %v",
			latestRevision, err), "componentName", componentName)
		return true, err
	}
	oldComp, err := util.UnpackRevisionData(oldRev)
	if err != nil {
		logger.Info(fmt.Sprintf("Unmarshal old controllerRevision %s error %v",
			latestRevision, err), "componentName", componentName)
		return true, err
	}
	if reflect.DeepEqual(curCompSpec, &oldComp.Spec) {
		// no need to create a new revision
		return false, nil
	}
	return true, nil
}

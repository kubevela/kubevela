/*
 Copyright 2021. The KubeVela Authors.

 Licensed under the Apache License, Version 2.0 (the "License");
 you may not use this file except in compliance with the License.
 You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

 Unless required by applicable law or agreed to in writing, software
 distributed under the License is distributed on an "AS IS" BASIS,
 WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 See the License for the specific language governing permissions and
 limitations under the License.
*/

package core

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/pkg/errors"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/controller/utils"
	"github.com/oam-dev/kubevela/pkg/oam"
)

// GenerateDefinitionRevision will generate a definition revision the generated revision
// will be compare with the last revision to see if there's any difference.
func GenerateDefinitionRevision(ctx context.Context, cli client.Client, def runtime.Object) (*v1beta1.DefinitionRevision, bool, error) {
	defRev, lastRevision, err := gatherRevisionInfo(def)
	if err != nil {
		return defRev, false, err
	}
	isNewRev, err := compareWithLastDefRevisionSpec(ctx, cli, defRev, lastRevision)
	if err != nil {
		return defRev, false, err
	}
	if isNewRev {
		defRevName, revNum := getDefNextRevision(defRev, lastRevision)
		defRev.Name = defRevName
		defRev.Spec.Revision = revNum
	}
	return defRev, isNewRev, nil
}

func gatherRevisionInfo(def runtime.Object) (*v1beta1.DefinitionRevision, *common.Revision, error) {
	defRev := &v1beta1.DefinitionRevision{}
	var LastRevision *common.Revision
	switch definition := def.(type) {
	case *v1beta1.ComponentDefinition:
		copiedCompDef := definition.DeepCopy()
		defRev.Spec.DefinitionType = common.ComponentType
		defRev.Spec.ComponentDefinition = *copiedCompDef
		LastRevision = copiedCompDef.Status.LatestRevision
	case *v1beta1.TraitDefinition:
		copiedTraitDef := definition.DeepCopy()
		defRev.Spec.DefinitionType = common.TraitType
		defRev.Spec.TraitDefinition = *copiedTraitDef
		LastRevision = copiedTraitDef.Status.LatestRevision
	default:
		return nil, nil, fmt.Errorf("unsupported type %v", definition)
	}
	defHash, err := computeDefinitionRevisionHash(defRev)
	if err != nil {
		return nil, nil, err
	}
	defRev.Spec.RevisionHash = defHash
	return defRev, LastRevision, nil
}

func computeDefinitionRevisionHash(defRev *v1beta1.DefinitionRevision) (string, error) {
	var defHash string
	var err error
	switch defRev.Spec.DefinitionType {
	case common.ComponentType:
		defHash, err = utils.ComputeSpecHash(&defRev.Spec.ComponentDefinition.Spec)
		if err != nil {
			return defHash, err
		}
	case common.TraitType:
		defHash, err = utils.ComputeSpecHash(&defRev.Spec.TraitDefinition.Spec)
		if err != nil {
			return defHash, err
		}
	}
	return defHash, nil
}

func compareWithLastDefRevisionSpec(ctx context.Context, cli client.Client,
	newDefRev *v1beta1.DefinitionRevision, lastRevision *common.Revision) (bool, error) {
	if lastRevision == nil {
		return true, nil
	}
	if lastRevision.RevisionHash != newDefRev.Spec.RevisionHash {
		return true, nil
	}

	// check if the DefinitionRevision is deep equal in Spec level
	// get the last revision from K8s and double check
	defRev := &v1beta1.DefinitionRevision{}
	var namespace string
	switch newDefRev.Spec.DefinitionType {
	case common.ComponentType:
		namespace = newDefRev.Spec.ComponentDefinition.Namespace
	case common.TraitType:
		namespace = newDefRev.Spec.TraitDefinition.Namespace
	}
	if err := cli.Get(ctx, client.ObjectKey{Name: lastRevision.Name,
		Namespace: namespace}, defRev); err != nil {
		return false, errors.Wrapf(err, "get the definitionRevision %s", lastRevision.Name)
	}
	if deepEqualDefRevision(defRev, newDefRev) {
		// No difference on spec, will not create a new revision
		// align the name and resourceVersion
		newDefRev.Name = defRev.Name
		newDefRev.ResourceVersion = defRev.ResourceVersion
		return false, nil
	}
	// if reach here, it's same hash but different spec
	return true, nil
}

func deepEqualDefRevision(old, new *v1beta1.DefinitionRevision) bool {
	if !apiequality.Semantic.DeepEqual(old.Spec.ComponentDefinition.Spec, new.Spec.ComponentDefinition.Spec) {
		return false
	}
	if !apiequality.Semantic.DeepEqual(old.Spec.TraitDefinition.Spec, new.Spec.TraitDefinition.Spec) {
		return false
	}
	return true
}

func getDefNextRevision(defRev *v1beta1.DefinitionRevision, lastRevision *common.Revision) (string, int64) {
	var nextRevision int64 = 1
	if lastRevision != nil {
		nextRevision = lastRevision.Revision + 1
	}
	var name string
	switch defRev.Spec.DefinitionType {
	case common.ComponentType:
		name = defRev.Spec.ComponentDefinition.Name
	case common.TraitType:
		name = defRev.Spec.TraitDefinition.Name
	}
	defRevName := strings.Join([]string{name, fmt.Sprintf("v%d", nextRevision)}, "-")
	return defRevName, nextRevision
}

// CleanUpDefinitionRevision check all definitionRevisions, remove them if the number of them exceed the limit
func CleanUpDefinitionRevision(ctx context.Context, cli client.Client, def runtime.Object, revisionLimit int) error {
	var listOpts []client.ListOption
	var usingRevision string

	switch definition := def.(type) {
	case *v1beta1.ComponentDefinition:
		listOpts = []client.ListOption{
			client.InNamespace(definition.Namespace),
			client.MatchingLabels{oam.LabelComponentDefinitionName: definition.Name},
		}
		usingRevision = definition.Status.LatestRevision.Name
	case *v1beta1.TraitDefinition:
		listOpts = []client.ListOption{
			client.InNamespace(definition.Namespace),
			client.MatchingLabels{oam.LabelTraitDefinitionName: definition.Name},
		}
		usingRevision = definition.Status.LatestRevision.Name
	}

	defRevList := new(v1beta1.DefinitionRevisionList)
	if err := cli.List(ctx, defRevList, listOpts...); err != nil {
		return err
	}
	needKill := len(defRevList.Items) - revisionLimit - 1
	if needKill <= 0 {
		return nil
	}
	klog.InfoS("cleanup old definitionRevision", "needKillNum", needKill)

	sortedRevision := defRevList.Items
	sort.Sort(historiesByRevision(sortedRevision))

	for _, rev := range sortedRevision {
		if needKill <= 0 {
			break
		}
		if rev.Name == usingRevision {
			continue
		}
		if err := cli.Delete(ctx, rev.DeepCopy()); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
		needKill--
	}
	return nil
}

type historiesByRevision []v1beta1.DefinitionRevision

func (h historiesByRevision) Len() int      { return len(h) }
func (h historiesByRevision) Swap(i, j int) { h[i], h[j] = h[j], h[i] }
func (h historiesByRevision) Less(i, j int) bool {
	return h[i].Spec.Revision < h[j].Spec.Revision
}

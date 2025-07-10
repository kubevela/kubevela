/*
Copyright 2021 The KubeVela Authors.

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

package utils

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"maps"
	"strconv"
	"strings"

	"github.com/mitchellh/hashstructure/v2"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/appfile"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

// GetAppNextRevision will generate the next revision name and revision number for application
func GetAppNextRevision(app *v1beta1.Application) (string, int64) {
	if app == nil {
		// should never happen
		return "", 0
	}
	var nextRevision int64 = 1
	if app.Status.LatestRevision != nil {
		// revision will always bump and increment no matter what the way user is running.
		nextRevision = app.Status.LatestRevision.Revision + 1
	}
	return ConstructRevisionName(app.Name, nextRevision), nextRevision
}

// ConstructRevisionName will generate a revisionName given the componentName and revision
// will be <componentName>-v<RevisionNumber>, for example: comp-v1
func ConstructRevisionName(componentName string, revision int64) string {
	return strings.Join([]string{componentName, fmt.Sprintf("v%d", revision)}, "-")
}

// ExtractComponentName will extract the componentName from a revisionName
var ExtractComponentName = util.ExtractComponentName

// ExtractRevision will extract the revision from a revisionName
func ExtractRevision(revisionName string) (int, error) {
	splits := strings.Split(revisionName, "-")
	// the revision is the last string without the prefix "v"
	return strconv.Atoi(strings.TrimPrefix(splits[len(splits)-1], "v"))
}

// ComputeSpecHash computes the hash value of a k8s resource spec
func ComputeSpecHash(spec interface{}) (string, error) {
	// compute a hash value of any resource spec
	specHash, err := hashstructure.Hash(spec, hashstructure.FormatV2, nil)
	if err != nil {
		return "", err
	}
	specHashLabel := strconv.FormatUint(specHash, 16)
	return specHashLabel, nil
}

// TODO:: add test coverage
func UpdateDefinitionRevisionLabels(af *appfile.Appfile, app *v1beta1.Application) error {
	if af.AppLabels == nil {
		af.AppLabels = make(map[string]string)
	} else {
		removeDefRevLabels(&af.AppLabels)
	}
	if app.ObjectMeta.Labels == nil {
		app.ObjectMeta.Labels = make(map[string]string)
	} else {
		removeDefRevLabels(&app.ObjectMeta.Labels)
	}

	// Add component, trait, workflow, and policy definition revision labels
	defRevLabels := make(map[string]string)

	for name, comp := range af.RelatedComponentDefinitions {
		defRevLabel, err := getDefRevLabel(name, common.ComponentType)
		if err != nil {
			return err
		}

		defRevLabels[defRevLabel] = fmt.Sprint(comp.GetGeneration())
	}

	for name, trait := range af.RelatedTraitDefinitions {
		defRevLabel, err := getDefRevLabel(name, common.TraitType)
		if err != nil {
			return err
		}

		defRevLabels[defRevLabel] = fmt.Sprint(trait.GetGeneration())
	}

	for name, wfStep := range af.RelatedWorkflowStepDefinitions {
		defRevLabel, err := getDefRevLabel(name, common.WorkflowStepType)
		if err != nil {
			return err
		}

		defRevLabels[defRevLabel] = fmt.Sprint(wfStep.GetGeneration())
	}

	for _, policy := range af.ParsedPolicies {
		defRevLabel, err := getDefRevLabel(policy.Name, common.WorkflowStepType)
		if err != nil {
			return err
		}

		defRevLabels[defRevLabel] = fmt.Sprint(policy.FullTemplate.PolicyDefinition.Generation)
	}

	maps.Copy(af.AppLabels, defRevLabels)
	maps.Copy(app.ObjectMeta.Labels, defRevLabels)

	return nil
}

// TODO:: add test coverage
func removeDefRevLabels(allLabels *map[string]string) {
	for key := range *allLabels {
		if strings.HasPrefix(key, oam.LabelComponentDefinitionRevision) ||
			strings.HasPrefix(key, oam.LabelTraitDefinitionRevision) ||
			strings.HasPrefix(key, oam.LabelWorkflowStepDefinitionRevision) ||
			strings.HasPrefix(key, oam.LabelPolicyDefinitionRevision) {
			delete(*allLabels, key)
		}
	}
}

// TODO:: add test coverage
func getDefRevLabel(defName string, defType common.DefinitionType) (string, error) {
	var labelPrefix, label string

	switch defType {
	case common.ComponentType:
		labelPrefix = oam.LabelComponentDefinitionRevision
	case common.PolicyType:
		labelPrefix = oam.LabelPolicyDefinitionRevision
	case common.TraitType:
		labelPrefix = oam.LabelTraitDefinitionRevision
	case common.WorkflowStepType:
		labelPrefix = oam.LabelWorkflowStepDefinitionRevision
	default:
		return "", fmt.Errorf("unknown definition type: %s", defType)
	}

	// NOTE:: assuming here that an app will never have multiple components of the same type and name
	label = fmt.Sprintf("%s-%s", labelPrefix, defName)

	// if the label is longer than 63 characters, truncate it by using its digest to maintain uniqueness
	if len(label) > 63 {
		sum := sha256.Sum256([]byte(label))
		suffix := hex.EncodeToString(sum[:])[:5]
		label = label[:len(label)-5] + suffix
	}

	return label, nil
}

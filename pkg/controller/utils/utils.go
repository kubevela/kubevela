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
	"fmt"
	"strconv"
	"strings"

	"github.com/mitchellh/hashstructure/v2"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
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

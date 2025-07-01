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

package utils

import (
	"context"
	"fmt"
	_ "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/pkg/oam"
	"os"
	"regexp"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	"sort"
	"strconv"
	"strings"

	"cuelang.org/go/cue/cuecontext"
	cueErrors "cuelang.org/go/cue/errors"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/v1beta1/core"
)

// ContextRegex to match '**: reference "context" not found'
var ContextRegex = `^.+:\sreference\s\"context\"\snot\sfound$`

// ValidateDefinitionRevision validate whether definition will modify the immutable object definitionRevision
func ValidateDefinitionRevision(ctx context.Context, cli client.Client, def runtime.Object, defRevNamespacedName types.NamespacedName) error {
	if errs := validation.IsQualifiedName(defRevNamespacedName.Name); len(errs) != 0 {
		return errors.Errorf("invalid definitionRevision name %s:%s", defRevNamespacedName.Name, strings.Join(errs, ","))
	}
	defRev := new(v1beta1.DefinitionRevision)
	if err := cli.Get(ctx, defRevNamespacedName, defRev); err != nil {
		return client.IgnoreNotFound(err)
	}

	newRev, _, err := core.GatherRevisionInfo(def)
	if err != nil {
		return err
	}
	if defRev.Spec.RevisionHash != newRev.Spec.RevisionHash {
		return errors.New("the definition's spec is different with existing definitionRevision's spec")
	}
	if !core.DeepEqualDefRevision(defRev, newRev) {
		return errors.New("the definition's spec is different with existing definitionRevision's spec")
	}
	return nil
}

// ValidateCueTemplate validate cueTemplate
func ValidateCueTemplate(cueTemplate string) error {

	val := cuecontext.New().CompileString(cueTemplate)
	if e := checkError(val.Err()); e != nil {
		return e
	}

	err := val.Validate()
	return checkError(err)
}

func checkError(err error) error {
	re := regexp.MustCompile(ContextRegex)
	if err != nil {
		// ignore context not found error
		for _, e := range cueErrors.Errors(err) {
			if !re.MatchString(e.Error()) {
				return cueErrors.New(e.Error())
			}
		}
	}
	return nil
}

// ValidateSemanticVersion validates if a Definition's version includes all of
// major,minor & patch version values.
func ValidateSemanticVersion(version string) error {
	if version != "" {
		versionParts := strings.Split(version, ".")
		if len(versionParts) != 3 {
			return errors.New("Not a valid version")
		}

		for _, versionPart := range versionParts {
			if _, err := strconv.Atoi(versionPart); err != nil {
				return errors.New("Not a valid version")
			}
		}
	}
	return nil
}

// ValidateMultipleDefVersionsNotPresent validates that both Name Annotation Revision and Spec.Version are not present
func ValidateMultipleDefVersionsNotPresent(version, revisionName, objectType string) error {
	if version != "" && revisionName != "" {
		return fmt.Errorf("%s has both spec.version and revision name annotation. Only one can be present", objectType)
	}
	return nil
}

// ValidateDefinitionRevisionCleanUp ensures that only a limited number of definition revisions are kept
// and verifies that revisions scheduled for cleanup are not currently in use by any applications.
//
// The function determines which definition revisions can be safely removed based on:
// 1. The configured revision limit
// 2. Whether any applications are using the oldest revisions
//
// If any applications are using the revision that would be deleted, the function returns an error
// to prevent breaking existing applications.
func ValidateDefinitionRevisionCleanUp(ctx context.Context, cli client.Client, req admission.Request) error {
	kindType := req.AdmissionRequest.Kind.Kind
	namespace := req.AdmissionRequest.Namespace
	name := req.AdmissionRequest.Name

	// Determine definition type and set appropriate list options
	listOpts, definitionPrefix, err := getDefinitionListOptions(kindType, namespace, name)
	if err != nil {
		return err
	}

	// Construct the definition name with prefix
	definitionName := definitionPrefix + "-" + name

	// List existing definition revisions
	defRevList := &v1beta1.DefinitionRevisionList{}
	if err := cli.List(ctx, defRevList, listOpts...); err != nil {
		return fmt.Errorf("failed to list %s revisions for %s: %w", kindType, name, err)
	}

	// Get configured revision limit
	revisionLimit, err := getRevisionLimitFromArgs()
	if err != nil {
		return fmt.Errorf("failed to get revision limit: %w", err)
	}

	// Calculate if any revisions need cleanup
	// We subtract 1 to account for the current revision that's being created
	totalRevisions := len(defRevList.Items)
	revisionsToDelete := totalRevisions - revisionLimit - 1

	// No cleanup needed if we're within limits
	if revisionsToDelete < 0 {
		return nil
	}

	// Sort revisions by revision number (oldest first)
	sortedRevisions := defRevList.Items
	sort.Sort(ByRevisionNumber(sortedRevisions))

	// Get the oldest revision that would be deleted
	oldestRevisionID := fmt.Sprintf("%d", sortedRevisions[0].Spec.Revision)

	// Check if any applications are using the revision scheduled for deletion
	appsUsingRevision := &v1beta1.ApplicationList{}
	if err := cli.List(ctx, appsUsingRevision,
		client.InNamespace(""), // Check across all namespaces
		client.MatchingLabels{definitionName: oldestRevisionID}); err != nil {
		return fmt.Errorf("failed to check applications using %s revision %s: %w",
			kindType, oldestRevisionID, err)
	}

	// If applications are using this revision, prevent deletion
	if len(appsUsingRevision.Items) > 0 {
		app := appsUsingRevision.Items[0]
		return fmt.Errorf("cannot apply new %s: application %s in namespace %s is using revision %s with definition %s",
			kindType, app.Name, app.Namespace, oldestRevisionID, definitionName)
	}

	return nil
}

// getDefinitionListOptions returns the appropriate list options and prefix based on definition kind.
func getDefinitionListOptions(kind, namespace, name string) ([]client.ListOption, string, error) {
	var listOpts []client.ListOption
	var prefix string

	switch kind {
	case "ComponentDefinition":
		listOpts = []client.ListOption{
			client.InNamespace(namespace),
			client.MatchingLabels{oam.LabelComponentDefinitionName: name},
		}
		prefix = "component"
	case "TraitDefinition":
		listOpts = []client.ListOption{
			client.InNamespace(namespace),
			client.MatchingLabels{oam.LabelTraitDefinitionName: name},
		}
		prefix = "trait"
	case "PolicyDefinition":
		listOpts = []client.ListOption{
			client.InNamespace(namespace),
			client.MatchingLabels{oam.LabelPolicyDefinitionName: name},
		}
		prefix = "policy"
	case "WorkFlowDefinition":
		listOpts = []client.ListOption{
			client.InNamespace(namespace),
			client.MatchingLabels{oam.LabelWorkflowStepDefinitionName: name},
		}
		prefix = "workflow"
	default:
		return nil, "", fmt.Errorf("unsupported definition kind: %s", kind)
	}

	return listOpts, prefix, nil
}

// getRevisionLimitFromArgs retrieves the definition revision limit from command line arguments.
// The limit determines how many older revisions of each definition should be retained.
func getRevisionLimitFromArgs() (int, error) {
	const argPrefix = "--definition-revision-limit="

	for _, arg := range os.Args[1:] {
		if strings.HasPrefix(arg, argPrefix) {
			valStr := strings.TrimPrefix(arg, argPrefix)
			limit, err := strconv.Atoi(valStr)
			if err != nil {
				return 0, fmt.Errorf("invalid revision limit value '%s': %w", valStr, err)
			}
			if limit < 0 {
				return 0, fmt.Errorf("revision limit cannot be negative: %d", limit)
			}
			return limit, nil
		}
	}

	return 0, fmt.Errorf("required argument %s not found", argPrefix)
}

// ByRevisionNumber implements sort.Interface for []v1beta1.DefinitionRevision
// to sort definition revisions by their revision number in ascending order.
type ByRevisionNumber []v1beta1.DefinitionRevision

func (r ByRevisionNumber) Len() int      { return len(r) }
func (r ByRevisionNumber) Swap(i, j int) { r[i], r[j] = r[j], r[i] }
func (r ByRevisionNumber) Less(i, j int) bool {
	return r[i].Spec.Revision < r[j].Spec.Revision
}

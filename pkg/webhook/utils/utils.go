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

func ValidateDefinitionRevisionCleanUp(ctx context.Context, cli client.Client, req admission.Request) error {
	var listOpts []client.ListOption

	// Set list options based on the definition kind using the appropriate label key.
	switch req.AdmissionRequest.Kind.Kind {
	case "ComponentDefinition":
		listOpts = []client.ListOption{
			client.InNamespace(req.AdmissionRequest.Namespace),
			client.MatchingLabels{oam.LabelComponentDefinitionName: req.AdmissionRequest.Name},
		}
	case "TraitDefinition":
		listOpts = []client.ListOption{
			client.InNamespace(req.AdmissionRequest.Namespace),
			client.MatchingLabels{oam.LabelTraitDefinitionName: req.AdmissionRequest.Name},
		}
	case "PolicyDefinition":
		listOpts = []client.ListOption{
			client.InNamespace(req.AdmissionRequest.Namespace),
			client.MatchingLabels{oam.LabelPolicyDefinitionName: req.AdmissionRequest.Name},
		}
	case "WorkFlowDefinition":
		listOpts = []client.ListOption{
			client.InNamespace(req.AdmissionRequest.Namespace),
			client.MatchingLabels{oam.LabelWorkflowStepDefinitionName: req.AdmissionRequest.Name},
		}
	default:
		return fmt.Errorf("unsupported kind %s", req.AdmissionRequest.Kind.Kind)
	}

	// List DefinitionRevisions matching the criteria.
	defRevList := new(v1beta1.DefinitionRevisionList)
	if err := cli.List(ctx, defRevList, listOpts...); err != nil {
		return fmt.Errorf("failed to list definition revisions: %w", err)
	}

	revisionLimit, err := getDefLimitFromArgs()
	if err != nil {
		return fmt.Errorf("failed to get revision limit: %w", err)
	}

	// Determine how many revisions exceed the limit.
	// Additional 1 subtracted to take care of the current revision in use.
	needKill := len(defRevList.Items) - revisionLimit - 1
	if needKill < 0 {
		return nil
	}

	// Sort the revisions by revision history.
	sortedRevision := defRevList.Items
	sort.Sort(historiesByRevision(sortedRevision))

	// Construct the component type name to be deleted.
	// Example format: "configmap-component@v11"
	componentRevToBeDeleted := fmt.Sprintf("%d", sortedRevision[0].Spec.Revision)
	componentNameToBeDeleted := "component-" + sortedRevision[0].Spec.ComponentDefinition.ObjectMeta.Name

	// Filter applications using the component type that's scheduled for deletion
	// List applications with the specific component revision as a label
	appWithComponentRev := new(v1beta1.ApplicationList)
	if err := cli.List(ctx, appWithComponentRev, client.InNamespace(""),
		client.MatchingLabels{
			componentNameToBeDeleted: componentRevToBeDeleted,
		}); err != nil {
		return fmt.Errorf("failed to list applications using component revision %s/%s: %w", componentNameToBeDeleted, componentRevToBeDeleted, err)
	}

	// If any applications are using this component revision, prevent deletion
	if len(appWithComponentRev.Items) > 0 {
		app := appWithComponentRev.Items[0]
		err := fmt.Errorf("could not apply new definition as application %s in namespace %s is already using revision %s with component %s",
			app.Name, app.Namespace, componentRevToBeDeleted, componentNameToBeDeleted)
		return err
	}
	return nil
}

func getDefLimitFromArgs() (int, error) {
	const prefix = "--definition-revision-limit="
	for _, arg := range os.Args[1:] {
		if strings.HasPrefix(arg, prefix) {
			valStr := strings.TrimPrefix(arg, prefix)
			limit, err := strconv.Atoi(valStr)
			if err != nil {
				return 0, fmt.Errorf("invalid %s value: %w", prefix, err)
			}
			return limit, nil
		}
	}
	return 0, fmt.Errorf("argument %s not found in os arguments", prefix)
}

type historiesByRevision []v1beta1.DefinitionRevision

func (h historiesByRevision) Len() int      { return len(h) }
func (h historiesByRevision) Swap(i, j int) { h[i], h[j] = h[j], h[i] }
func (h historiesByRevision) Less(i, j int) bool {
	return h[i].Spec.Revision < h[j].Spec.Revision
}

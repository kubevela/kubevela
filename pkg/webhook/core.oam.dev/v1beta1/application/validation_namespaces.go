/*
Copyright 2026 The KubeVela Authors.

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

package application

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	apitypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation/field"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/definition/nsrestrict"
	"github.com/oam-dev/kubevela/pkg/features"
	oamutil "github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/pkg/workflow/step"
)

// ValidateDefinitionNamespaces rejects an Application that uses a definition
// restricted to other namespaces.
//
// Admission is the only place this is enforced. An Application admitted before a
// restriction existed keeps reconciling, but cannot be edited until it moves or
// the restriction widens, because this runs on update as well as create.
//
// Costs one Get per distinct definition type.
func (h *ValidatingHandler) ValidateDefinitionNamespaces(ctx context.Context, app *v1beta1.Application) field.ErrorList {
	if !utilfeature.DefaultMutableFeatureGate.Enabled(features.RestrictDefinitionNamespaces) {
		return nil
	}

	var errs field.ErrorList
	usage := collectDefinitionUsage(app)
	defCtx := oamutil.SetNamespaceInCtx(ctx, app.Namespace)

	// Read the Application's namespace only when a definition selects on labels,
	// so restrictions by name cost no extra API calls.
	var nsLabels map[string]string
	var nsErr error
	var nsLabelsLoaded bool
	namespaceLabels := func() (map[string]string, error) {
		if nsLabelsLoaded {
			return nsLabels, nsErr
		}
		nsLabelsLoaded = true
		ns := &corev1.Namespace{}
		if err := h.namespaceReader().Get(ctx, apitypes.NamespacedName{Name: app.Namespace}, ns); err != nil {
			klog.Errorf("Failed to read namespace %q to check a definition's namespace selector: %v", app.Namespace, err)
			nsErr = err
			return nil, err
		}
		// An empty map still matches a selector that requires nothing; nil, returned
		// above on error, matches none.
		nsLabels = ns.Labels
		if nsLabels == nil {
			nsLabels = map[string]string{}
		}
		return nsLabels, nil
	}

	check := func(defType string, newDef func() client.Object, paths []*field.Path) {
		// Read the definition itself, never a DefinitionRevision: webservice@v1 names
		// the definition "webservice" rendered from a frozen revision, so strip the
		// pin and the current restriction applies, whatever revision is pinned.
		//
		// This is the cached client, so a restriction written moments ago may not be
		// visible yet. Definitions are already cached for the controller, which makes
		// the read free; an uncached one would cost an API call per definition type
		// on every Application admission.
		name := baseDefinitionName(defType)
		def := newDef()
		if err := oamutil.GetDefinition(defCtx, h.Client, def, name); err != nil {
			if !errors.IsNotFound(err) {
				klog.Errorf("Failed to load definition %q to check its namespace restriction: %v", name, err)
			}
			// ValidateComponents reports a missing or unreadable definition.
			return
		}
		var labels map[string]string
		if nsrestrict.NeedsNamespaceLabels(def, app.Namespace) {
			var nsReadErr error
			if labels, nsReadErr = namespaceLabels(); nsReadErr != nil {
				// A selector cannot be evaluated without the labels, so the
				// Application is still refused, but as a retryable server-side
				// failure rather than a policy violation.
				for _, p := range paths {
					errs = append(errs, field.InternalError(p, fmt.Errorf(
						"cannot evaluate the namespace restriction on %s %q: reading namespace %q: %w",
						nsrestrict.KindOf(def), name, app.Namespace, nsReadErr)))
				}
				return
			}
		}
		if err := nsrestrict.Check(def, app.Namespace, labels); err != nil {
			// The operator gets the patterns, through the log. The Application's
			// author gets only that the definition is restricted.
			klog.Infof("Denied %s %q to Application %q in namespace %q: allowed for %s",
				nsrestrict.KindOf(def), name, app.Name, app.Namespace, nsrestrict.Describe(def))
			for _, p := range paths {
				errs = append(errs, field.Forbidden(p, err.Error()))
			}
		}
	}

	for defType, indices := range usage.componentTypes {
		check(defType, func() client.Object { return &v1beta1.ComponentDefinition{} },
			componentTypePaths(indices))
	}
	for defType, locations := range usage.traitTypes {
		check(defType, func() client.Object { return &v1beta1.TraitDefinition{} },
			traitTypePaths(locations))
	}
	for defType, indices := range usage.policyTypes {
		check(defType, func() client.Object { return &v1beta1.PolicyDefinition{} },
			policyTypePaths(indices))
	}
	for defType, locations := range usage.workflowStepTypes {
		// A builtin step has no definition to restrict.
		if step.IsBuiltinWorkflowStepType(defType) {
			continue
		}
		check(defType, func() client.Object { return &v1beta1.WorkflowStepDefinition{} },
			workflowStepTypePaths(locations))
	}
	for defType, indices := range usage.sourceTypes {
		check(defType, func() client.Object { return &v1beta1.SourceDefinition{} },
			sourceTypePaths(indices))
	}

	return errs
}

func componentTypePaths(indices []int) []*field.Path {
	paths := make([]*field.Path, 0, len(indices))
	for _, idx := range indices {
		paths = append(paths, field.NewPath("spec", "components").Index(idx).Child("type"))
	}
	return paths
}

func traitTypePaths(locations [][2]int) []*field.Path {
	paths := make([]*field.Path, 0, len(locations))
	for _, loc := range locations {
		paths = append(paths,
			field.NewPath("spec", "components").Index(loc[0]).Child("traits").Index(loc[1]).Child("type"))
	}
	return paths
}

func policyTypePaths(indices []int) []*field.Path {
	paths := make([]*field.Path, 0, len(indices))
	for _, idx := range indices {
		paths = append(paths, field.NewPath("spec", "policies").Index(idx).Child("type"))
	}
	return paths
}

func workflowStepTypePaths(locations []workflowStepLocation) []*field.Path {
	paths := make([]*field.Path, 0, len(locations))
	for _, loc := range locations {
		paths = append(paths, getWorkflowStepFieldPath(loc))
	}
	return paths
}

func sourceTypePaths(indices []int) []*field.Path {
	paths := make([]*field.Path, 0, len(indices))
	for _, idx := range indices {
		paths = append(paths, field.NewPath("spec", "sources").Index(idx).Child("type"))
	}
	return paths
}

// namespaceReader prefers the uncached reader, so a single lookup does not pull
// every Namespace into the controller's cache.
func (h *ValidatingHandler) namespaceReader() client.Reader {
	if h.APIReader != nil {
		return h.APIReader
	}
	return h.Client
}

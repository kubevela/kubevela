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

package appfile

import (
	"context"
	"fmt"
	"strings"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/util/feature"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/cue/definition/health"
	"github.com/oam-dev/kubevela/pkg/definition/inherit"
	"github.com/oam-dev/kubevela/pkg/features"
	oamutil "github.com/oam-dev/kubevela/pkg/oam/util"
)

// MaxInheritanceDepth caps how far a chain of `extends` may run. A backstop
// rather than a cost control: rendering stays linear in the depth, but a
// definition assembled from more places than this is not one anybody can
// reason about.
const MaxInheritanceDepth = 8

// componentFetcher resolves a ComponentDefinition by the name written in
// spec.extends, which may name a DefinitionRevision as "webservice@v3".
type componentFetcher func(ctx context.Context, name string) (*v1beta1.ComponentDefinition, error)

// traitFetcher does the same for a TraitDefinition.
type traitFetcher func(ctx context.Context, name string) (*v1beta1.TraitDefinition, error)

// resolveComponentChain walks `extends` and records what it found on the
// Template.
//
// Ancestors are kept as definitions rather than templates because the
// ApplicationRevision stores them, which is what makes a chain reproducible.
// They are keyed by the name the child wrote, so `webservice@v3` and plain
// `webservice` keep separate entries.
func resolveComponentChain(ctx context.Context, tmpl *Template, cd *v1beta1.ComponentDefinition, fetch componentFetcher) error {
	if cd == nil || cd.Spec.Extends == "" {
		return nil
	}
	if err := inheritanceEnabled(cd.Name, cd.Spec.Extends); err != nil {
		return err
	}

	seen := []string{cd.Name}
	current := cd
	for depth := 0; current.Spec.Extends != ""; depth++ {
		name := current.Spec.Extends
		if err := checkChain(seen, name, depth, current.Name); err != nil {
			return err
		}
		parent, err := fetch(ctx, name)
		if err != nil {
			return fmt.Errorf("component definition %s extends %s: %w", current.Name, name, err)
		}
		if parent.Spec.Schematic == nil || parent.Spec.Schematic.CUE == nil {
			return fmt.Errorf(
				"component definition %s extends %s, which has no CUE template; only CUE definitions can be extended",
				current.Name, name)
		}
		tmpl.Ancestors = append(tmpl.Ancestors, inherit.Level{Name: name, Template: parent.Spec.Schematic.CUE.Template})
		if tmpl.AncestorComponentDefinitions == nil {
			tmpl.AncestorComponentDefinitions = map[string]*v1beta1.ComponentDefinition{}
		}
		stored := parent.DeepCopy()
		stored.Status = v1beta1.ComponentDefinitionStatus{}
		tmpl.AncestorComponentDefinitions[name] = stored

		inheritComponentAttributes(tmpl, current, parent)
		seen = append(seen, name)
		current = parent
	}
	return nil
}

// resolveTraitChain is resolveComponentChain for traits.
func resolveTraitChain(ctx context.Context, tmpl *Template, td *v1beta1.TraitDefinition, fetch traitFetcher) error {
	if td == nil || td.Spec.Extends == "" {
		return nil
	}
	if err := inheritanceEnabled(td.Name, td.Spec.Extends); err != nil {
		return err
	}

	seen := []string{td.Name}
	current := td
	for depth := 0; current.Spec.Extends != ""; depth++ {
		name := current.Spec.Extends
		if err := checkChain(seen, name, depth, current.Name); err != nil {
			return err
		}
		parent, err := fetch(ctx, name)
		if err != nil {
			return fmt.Errorf("trait definition %s extends %s: %w", current.Name, name, err)
		}
		if parent.Spec.Schematic == nil || parent.Spec.Schematic.CUE == nil {
			return fmt.Errorf(
				"trait definition %s extends %s, which has no CUE template; only CUE definitions can be extended",
				current.Name, name)
		}
		tmpl.Ancestors = append(tmpl.Ancestors, inherit.Level{Name: name, Template: parent.Spec.Schematic.CUE.Template})
		if tmpl.AncestorTraitDefinitions == nil {
			tmpl.AncestorTraitDefinitions = map[string]*v1beta1.TraitDefinition{}
		}
		stored := parent.DeepCopy()
		stored.Status = v1beta1.TraitDefinitionStatus{}
		tmpl.AncestorTraitDefinitions[name] = stored

		inheritTraitAttributes(tmpl, current, parent)
		seen = append(seen, name)
		current = parent
	}
	return nil
}

// checkChain refuses a chain that loops or runs too deep. A cycle is caught by
// name rather than left to recurse until something else gives out.
func checkChain(seen []string, next string, depth int, from string) error {
	if depth >= MaxInheritanceDepth {
		return fmt.Errorf(
			"definition %s extends %s, which would make the inheritance chain deeper than %d: %s",
			from, next, MaxInheritanceDepth, strings.Join(append(seen, next), " -> "))
	}
	for _, s := range seen {
		if s == next || baseName(s) == baseName(next) {
			return fmt.Errorf(
				"definition %s extends %s, which is already in its own inheritance chain: %s",
				from, next, strings.Join(append(seen, next), " -> "))
		}
	}
	return nil
}

// baseName strips a pinned revision, so "webservice@v3" and "webservice" count
// as the same definition when looking for a cycle.
func baseName(name string) string {
	if i := strings.LastIndex(name, "@"); i > 0 {
		return name[:i]
	}
	return name
}

// inheritanceEnabled refuses `extends` while the feature is off. Ignoring the
// field would render the child's template with an unresolved `$super` and fail
// somewhere unrelated.
func inheritanceEnabled(name, extends string) error {
	if feature.DefaultMutableFeatureGate.Enabled(features.EnableDefinitionInheritance) {
		return nil
	}
	return fmt.Errorf(
		"definition %s sets spec.extends: %s, but definition inheritance is disabled; "+
			"start the controller with --feature-gates=EnableDefinitionInheritance=true to use it",
		name, extends)
}

// inheritComponentAttributes fills in what the child left empty. Only the
// template is composed by rendering; the rest of the spec describes the
// workload, where silence means the parent's answer.
func inheritComponentAttributes(tmpl *Template, child, parent *v1beta1.ComponentDefinition) {
	if child.Spec.Workload.Definition.Kind == "" && child.Spec.Workload.Type == "" {
		child.Spec.Workload = parent.Spec.Workload
		tmpl.Reference = parent.Spec.Workload
	}
	if child.Spec.PodSpecPath == "" {
		child.Spec.PodSpecPath = parent.Spec.PodSpecPath
	}
	if child.Spec.RevisionLabel == "" {
		child.Spec.RevisionLabel = parent.Spec.RevisionLabel
	}
	if len(child.Spec.ChildResourceKinds) == 0 {
		child.Spec.ChildResourceKinds = parent.Spec.ChildResourceKinds
	}
	tmpl.AncestorStatus = append(tmpl.AncestorStatus, snippetsOf(parent.Spec.Status))
}

// snippetsOf reads a definition's status CUE, which composes at evaluation time
// rather than being folded into the child here: a child that states a status
// message has not thereby given up its parent's health policy.
func snippetsOf(status *common.Status) health.Snippets {
	if status == nil {
		return health.Snippets{}
	}
	return health.Snippets{
		Health:  status.HealthPolicy,
		Custom:  status.CustomStatus,
		Details: status.Details,
	}
}

// inheritTraitAttributes is inheritComponentAttributes for traits.
//
// The booleans are OR'd rather than inherited when empty, since a Go bool cannot
// tell unset from false. A child therefore cannot turn off a parent's
// podDisruptive.
func inheritTraitAttributes(tmpl *Template, child, parent *v1beta1.TraitDefinition) {
	if len(child.Spec.AppliesToWorkloads) == 0 {
		child.Spec.AppliesToWorkloads = parent.Spec.AppliesToWorkloads
	}
	if len(child.Spec.ConflictsWith) == 0 {
		child.Spec.ConflictsWith = parent.Spec.ConflictsWith
	}
	if child.Spec.WorkloadRefPath == "" {
		child.Spec.WorkloadRefPath = parent.Spec.WorkloadRefPath
	}
	if child.Spec.Stage == "" {
		child.Spec.Stage = parent.Spec.Stage
	}
	child.Spec.PodDisruptive = child.Spec.PodDisruptive || parent.Spec.PodDisruptive
	child.Spec.RevisionEnabled = child.Spec.RevisionEnabled || parent.Spec.RevisionEnabled
	child.Spec.ManageWorkload = child.Spec.ManageWorkload || parent.Spec.ManageWorkload
	child.Spec.ControlPlaneOnly = child.Spec.ControlPlaneOnly || parent.Spec.ControlPlaneOnly

	tmpl.AncestorStatus = append(tmpl.AncestorStatus, snippetsOf(parent.Spec.Status))
}

// clusterComponentFetcher reads a parent from the cluster, honouring a pinned
// revision in the name, and only from the child's own namespace.
func clusterComponentFetcher(cli client.Client, namespace string, annotations map[string]string) componentFetcher {
	return func(ctx context.Context, name string) (*v1beta1.ComponentDefinition, error) {
		cd := new(v1beta1.ComponentDefinition)
		if err := oamutil.GetCapabilityDefinition(inNamespace(ctx, namespace), cli, cd, name, annotations); err != nil {
			return nil, err
		}
		return cd, sameNamespace("component", cd.Name, cd.Namespace, namespace)
	}
}

// clusterTraitFetcher is clusterComponentFetcher for traits.
func clusterTraitFetcher(cli client.Client, namespace string, annotations map[string]string) traitFetcher {
	return func(ctx context.Context, name string) (*v1beta1.TraitDefinition, error) {
		td := new(v1beta1.TraitDefinition)
		if err := oamutil.GetCapabilityDefinition(inNamespace(ctx, namespace), cli, td, name, annotations); err != nil {
			return nil, err
		}
		return td, sameNamespace("trait", td.Name, td.Namespace, namespace)
	}
}

// inNamespace points a capability lookup at one namespace first.
func inNamespace(ctx context.Context, namespace string) context.Context {
	return oamutil.SetNamespaceInCtx(ctx, namespace)
}

// sameNamespace refuses a parent that came from anywhere but the child's own
// namespace.
//
// A capability lookup falls back to the x-definition and system namespaces, so
// asking for a parent in one namespace can return one from another. Extending
// across that boundary would let a definition in a user's namespace render a
// privileged one from vela-system, and the Application permission check would
// see only the child. Confining a chain to one namespace also means admission,
// the definition controller and the render path all resolve it identically,
// which they cannot while the namespace comes from an ambient context.
//
// Only a chain being resolved outside any namespace is exempt, which is what
// `vela dry-run` and `vela show` do when they read definitions from files. A
// parent that arrives from the cluster without a namespace is a cluster-scoped
// install predating namespaced definitions: global, admin-installed, and exactly
// the sort of thing a team's namespace should not be able to build on.
func sameNamespace(kind, name, found, want string) error {
	if want == "" || found == want {
		return nil
	}
	if found == "" {
		return fmt.Errorf(
			"%s definition %s is cluster-scoped, and a definition may only extend one in its own namespace (%s); "+
				"copy it into %s",
			kind, name, want, want)
	}
	return fmt.Errorf(
		"%s definition %s is in namespace %s, and a definition may only extend one in its own namespace (%s); "+
			"copy it into %s, or put the extending definition alongside it",
		kind, name, found, want, want)
}

// revisionComponentFetcher reads a parent from the ApplicationRevision and from
// nowhere else. Falling back to the cluster would mean a replayed revision
// picked up whatever the parent has since become, which is the drift the
// revision exists to prevent.
func revisionComponentFetcher(apprev *v1beta1.ApplicationRevision) componentFetcher {
	return func(_ context.Context, name string) (*v1beta1.ComponentDefinition, error) {
		cd, ok := apprev.Spec.ComponentDefinitions[name]
		if !ok || cd == nil {
			return nil, fmt.Errorf(
				"component definition %s not found in app revision %s; "+
					"the revision was taken before this definition extended anything, "+
					"or its ancestors were not recorded with it", name, apprev.Name)
		}
		return cd.DeepCopy(), nil
	}
}

// revisionTraitFetcher is revisionComponentFetcher for traits.
func revisionTraitFetcher(apprev *v1beta1.ApplicationRevision) traitFetcher {
	return func(_ context.Context, name string) (*v1beta1.TraitDefinition, error) {
		td, ok := apprev.Spec.TraitDefinitions[name]
		if !ok || td == nil {
			return nil, fmt.Errorf(
				"trait definition %s not found in app revision %s; "+
					"the revision was taken before this definition extended anything, "+
					"or its ancestors were not recorded with it", name, apprev.Name)
		}
		return td.DeepCopy(), nil
	}
}

// localComponentFetcher prefers definitions handed to a dry run, so a chain can
// be exercised before any of it is applied.
func localComponentFetcher(defs []*unstructured.Unstructured, cli client.Client, namespace string, annotations map[string]string) componentFetcher {
	return func(ctx context.Context, name string) (*v1beta1.ComponentDefinition, error) {
		for _, def := range defs {
			if def.GetKind() != v1beta1.ComponentDefinitionKind || def.GetName() != name {
				continue
			}
			cd := &v1beta1.ComponentDefinition{}
			if err := runtime.DefaultUnstructuredConverter.FromUnstructured(def.Object, cd); err != nil {
				return nil, errors.Wrapf(err, "invalid component definition %s", name)
			}
			// A file carries no namespace and is being dry-run into this one, so
			// there is nothing to confine. One that names a namespace is held to
			// it, or a dry run would pass a chain the cluster refuses.
			if cd.Namespace == "" {
				return cd, nil
			}
			return cd, sameNamespace("component", cd.Name, cd.Namespace, namespace)
		}
		return clusterComponentFetcher(cli, namespace, annotations)(ctx, name)
	}
}

// localTraitFetcher is localComponentFetcher for traits.
func localTraitFetcher(defs []*unstructured.Unstructured, cli client.Client, namespace string, annotations map[string]string) traitFetcher {
	return func(ctx context.Context, name string) (*v1beta1.TraitDefinition, error) {
		for _, def := range defs {
			if def.GetKind() != v1beta1.TraitDefinitionKind || def.GetName() != name {
				continue
			}
			td := &v1beta1.TraitDefinition{}
			if err := runtime.DefaultUnstructuredConverter.FromUnstructured(def.Object, td); err != nil {
				return nil, errors.Wrapf(err, "invalid trait definition %s", name)
			}
			if td.Namespace == "" {
				return td, nil
			}
			return td, sameNamespace("trait", td.Name, td.Namespace, namespace)
		}
		return clusterTraitFetcher(cli, namespace, annotations)(ctx, name)
	}
}

// ComponentAncestors resolves what a ComponentDefinition extends, for a caller
// wanting the chain rather than a Template. Admission uses it, so a broken chain
// is refused when the definition is written.
func ComponentAncestors(ctx context.Context, cli client.Client, cd *v1beta1.ComponentDefinition) ([]inherit.Level, error) {
	tmpl := &Template{}
	if err := resolveComponentChain(ctx, tmpl, cd.DeepCopy(), clusterComponentFetcher(cli, cd.Namespace, cd.Annotations)); err != nil {
		return nil, err
	}
	return tmpl.Ancestors, nil
}

// TraitAncestors is ComponentAncestors for a TraitDefinition.
func TraitAncestors(ctx context.Context, cli client.Client, td *v1beta1.TraitDefinition) ([]inherit.Level, error) {
	tmpl := &Template{}
	if err := resolveTraitChain(ctx, tmpl, td.DeepCopy(), clusterTraitFetcher(cli, td.Namespace, td.Annotations)); err != nil {
		return nil, err
	}
	return tmpl.Ancestors, nil
}

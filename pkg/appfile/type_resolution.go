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

package appfile

import (
	"context"
	"fmt"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/module/naming"
	"github.com/oam-dev/kubevela/pkg/oam"
	oamutil "github.com/oam-dev/kubevela/pkg/oam/util"
)

// parseTypeRef parses a component type string into its KEP-2.20 form (1, 2, or
// 3) and identity fields. Returns an error for an invalid two-segment string
// (first segment does not match ^v\d+) and for strings with four or more
// segments.
//
// Form 1: "bucket"         → form=1, name="bucket"
// Form 2: "v1/bucket"      → form=2, apiVersion="v1", name="bucket"
// Form 3: "s3/v1/bucket"   → form=3, module="s3", apiVersion="v1", name="bucket"
func parseTypeRef(typeName string) (form int, module, apiVersion, name string, err error) {
	parts := strings.Split(typeName, "/")
	switch len(parts) {
	case 1:
		return 1, "", "", parts[0], nil
	case 2:
		if !naming.IsValidAPIVersion(parts[0]) {
			return 0, "", "", "", fmt.Errorf(
				"invalid type reference %q: %q is not a valid API version; "+
					"expected v<N>, v<N>alpha<N>, or v<N>beta<N> (e.g. v1, v1alpha1, v2beta2). "+
					"For a module-scoped reference use {module}/{apiVersion}/{name} (e.g. s3/v1/bucket)",
				typeName, parts[0])
		}
		return 2, "", parts[0], parts[1], nil
	case 3:
		return 3, parts[0], parts[1], parts[2], nil
	default:
		return 0, "", "", "", fmt.Errorf(
			"invalid type reference %q: expected 1 segment (name), 2 segments (v<N>/name), "+
				"or 3 segments (module/v<N>/name)",
			typeName)
	}
}

// ResolveModuleType translates a KEP-2.20 Form 1/2/3 type string to the
// installed Kubernetes definition name. Form 1 names that already exist as
// legacy definitions are returned unchanged so the existing
// GetCapabilityDefinition path handles them as before.
func ResolveModuleType(ctx context.Context, cli client.Reader, typeName string, capType types.CapType) (string, error) {
	form, mod, apiVersion, name, err := parseTypeRef(typeName)
	if err != nil {
		return "", err
	}
	switch form {
	case 3:
		return resolveForm3(mod, apiVersion, name), nil
	case 2:
		return resolveForm2(ctx, cli, typeName, apiVersion, name, capType)
	default:
		return resolveForm1(ctx, cli, typeName, name, capType)
	}
}

// resolveForm3 derives the installed name from the three segments. The
// derivation is shared with the render service through pkg/module/naming, so
// this requires no cluster call and cannot drift from what was installed.
func resolveForm3(module, apiVersion, name string) string {
	return naming.DefinitionName(module, apiVersion, name)
}

// resolveForm2 resolves a version-scoped reference via label selector across
// the app namespace and vela-system, aggregating results before deciding.
func resolveForm2(ctx context.Context, cli client.Reader, typeName, apiVersion, name string, capType types.CapType) (string, error) {
	matches, err := listModuleDefinitions(ctx, cli, capType, map[string]string{
		types.LabelDefinitionModuleAPIVersion: apiVersion,
		types.LabelDefinitionName:             name,
	})
	if err != nil {
		return "", fmt.Errorf("resolving type %q: %w", typeName, err)
	}
	return resolveUnique(typeName, matches, "use a fully qualified type ({module}/{apiVersion}/{name}) to disambiguate")
}

// resolveForm1 first tries a plain GET (legacy path). On not-found it falls
// back to a label-selector search across the app namespace and vela-system.
func resolveForm1(ctx context.Context, cli client.Reader, typeName, name string, capType types.CapType) (string, error) {
	obj := definitionObjectFor(capType)
	if err := oamutil.GetDefinition(ctx, cli, obj, name); err == nil {
		// Found as a legacy definition; return unchanged so the caller's
		// GetCapabilityDefinition path (which handles revisions etc.) runs normally.
		return typeName, nil
	} else if !apierrors.IsNotFound(err) {
		return "", err
	}

	// Not a legacy definition; try the module label fallback. The search is
	// best-effort: a bare name resolved fine before modules existed, so a failed
	// search must not turn a plain "definition not found" into a different error.
	// Return the name unchanged and let the caller's own lookup report the
	// authoritative result, exactly as the no-matches case below does.
	matches, err := listModuleDefinitions(ctx, cli, capType, map[string]string{
		types.LabelDefinitionName: name,
	})
	if err != nil {
		klog.V(4).InfoS("module label search failed, falling back to the plain definition lookup",
			"type", typeName, "err", err)
		return typeName, nil
	}
	if len(matches) == 0 {
		// Surface the original not-found via the normal GetCapabilityDefinition path.
		return typeName, nil
	}
	return resolveUnique(typeName, matches, "use {apiVersion}/{name} (Form 2) or {module}/{apiVersion}/{name} (Form 3) to disambiguate")
}

// listModuleDefinitions lists definitions matching the label selector across the
// app namespace and vela-system, deduplicating when the two are the same.
func listModuleDefinitions(ctx context.Context, cli client.Reader, capType types.CapType, matchLabels map[string]string) ([]client.Object, error) {
	appNs := oamutil.GetDefinitionNamespaceWithCtx(ctx)
	systemNs := oam.SystemDefinitionNamespace
	namespaces := []string{appNs}
	if appNs != systemNs {
		namespaces = append(namespaces, systemNs)
	}

	var all []client.Object
	for _, ns := range namespaces {
		opts := []client.ListOption{
			client.InNamespace(ns),
			client.MatchingLabels(matchLabels),
		}
		switch capType {
		case types.TypeComponentDefinition, types.TypeWorkload:
			l := &v1beta1.ComponentDefinitionList{}
			if err := cli.List(ctx, l, opts...); err != nil {
				return nil, err
			}
			for i := range l.Items {
				all = append(all, &l.Items[i])
			}
		case types.TypeTrait:
			l := &v1beta1.TraitDefinitionList{}
			if err := cli.List(ctx, l, opts...); err != nil {
				return nil, err
			}
			for i := range l.Items {
				all = append(all, &l.Items[i])
			}
		case types.TypePolicy:
			l := &v1beta1.PolicyDefinitionList{}
			if err := cli.List(ctx, l, opts...); err != nil {
				return nil, err
			}
			for i := range l.Items {
				all = append(all, &l.Items[i])
			}
		case types.TypeWorkflowStep:
			l := &v1beta1.WorkflowStepDefinitionList{}
			if err := cli.List(ctx, l, opts...); err != nil {
				return nil, err
			}
			for i := range l.Items {
				all = append(all, &l.Items[i])
			}
		default:
			return nil, fmt.Errorf("unsupported capType %q for module type resolution", capType)
		}
	}
	return all, nil
}

// resolveUnique returns the single match's Kubernetes name, or an appropriate error
// for zero or more-than-one matches.
func resolveUnique(typeName string, matches []client.Object, disambiguateHint string) (string, error) {
	switch len(matches) {
	case 1:
		return matches[0].GetName(), nil
	case 0:
		return "", fmt.Errorf("no definition found for type %q", typeName)
	default:
		mods := make([]string, 0, len(matches))
		seen := map[string]bool{}
		for _, m := range matches {
			mod := m.GetLabels()[types.LabelDefinitionModule]
			if mod == "" {
				mod = m.GetName()
			}
			if !seen[mod] {
				mods = append(mods, mod)
				seen[mod] = true
			}
		}
		return "", fmt.Errorf(
			"type %q is ambiguous: definitions from modules [%s] all match; %s",
			typeName, strings.Join(mods, ", "), disambiguateHint)
	}
}

// definitionObjectFor returns a zero-value typed object for the given capType,
// used by resolveForm1 when doing a plain GET to check for a legacy definition.
func definitionObjectFor(capType types.CapType) client.Object {
	switch capType {
	case types.TypeTrait:
		return new(v1beta1.TraitDefinition)
	case types.TypePolicy:
		return new(v1beta1.PolicyDefinition)
	case types.TypeWorkflowStep:
		return new(v1beta1.WorkflowStepDefinition)
	default:
		return new(v1beta1.ComponentDefinition)
	}
}

// DefinitionExists verifies that the resolved definition name exists in the app
// namespace or vela-system. Used by the webhook to catch Form 3 references
// where resolution is pure string math and does not touch the cluster.
func DefinitionExists(ctx context.Context, cli client.Reader, name string, capType types.CapType) error {
	obj := definitionObjectFor(capType)
	if err := oamutil.GetDefinition(ctx, cli, obj, name); err != nil {
		if apierrors.IsNotFound(err) {
			return fmt.Errorf("definition %q not found: ensure the module is installed", name)
		}
		return err
	}
	return nil
}

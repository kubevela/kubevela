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

package addon

import (
	"encoding/json"
	"fmt"
	"slices"
	"strconv"

	"cuelang.org/go/cue/cuecontext"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"

	common2 "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/cue/nesting"
	"github.com/oam-dev/kubevela/pkg/features"
	"github.com/oam-dev/kubevela/pkg/policy"
	"github.com/oam-dev/kubevela/pkg/policy/placement"
)

// CrateImport is one crates/_imports.cue entry: an external crate the
// addon references by name, resolved from a crate registry when the addon's
// rendered type: crate component reconciles.
type CrateImport struct {
	// Crate is the crate name; also the emitted component's name.
	Crate string
	// Enabled gates whether this import is installed. Absent in the source
	// file defaults to true.
	Enabled bool
	// Registry is the named crate registry to resolve Crate from; empty
	// means search every configured registry.
	Registry string
	// Version is the exact crate package version to install; empty means
	// latest. addon:build/addon:publish already reject a semver range here,
	// so this value is trusted as an exact pin by the time it reaches render time.
	Version string
	// Placement is where the crate's runtime objects go. Nil means the entry
	// sets no placement, so the component carries none, as before.
	Placement *ImportPlacement
}

// ImportPlacement is an import's placement field. Either FromPolicy names one
// of the addon Application's own topology policies, resolved when the
// component is rendered, or the literal fields are copied as they are.
type ImportPlacement struct {
	Clusters             []string
	ClusterLabelSelector map[string]string
	AllowEmpty           bool
	FromPolicy           string
}

// rawCrateImports is the crates/_imports.cue file shape.
type rawCrateImports struct {
	Imports []rawCrateImport `json:"imports"`
}

// rawCrateImport is one raw imports[] entry before validation/flattening.
type rawCrateImport struct {
	Crate     string           `json:"crate"`
	Enabled   *bool            `json:"enabled"`
	Sources   []rawCrateSource `json:"sources"`
	Placement *rawPlacement    `json:"placement"`
}

// rawPlacement is one raw placement field. AllowEmpty is a pointer so a
// fromPolicy entry that also sets it can be told apart from one that does not.
type rawPlacement struct {
	Clusters             []string          `json:"clusters"`
	ClusterLabelSelector map[string]string `json:"clusterLabelSelector"`
	AllowEmpty           *bool             `json:"allowEmpty"`
	FromPolicy           string            `json:"fromPolicy"`
}

// rawCrateSource is one raw sources[] entry. Only registry/version/versions
// are read; oci/git/apiLine sources are not supported in this story.
type rawCrateSource struct {
	Registry string   `json:"registry"`
	Version  string   `json:"version"`
	Versions []string `json:"versions"`
}

// parseCrateImports parses a crates/_imports.cue file's contents into its
// CrateImport entries. Every entry must declare exactly one sources[] entry
// _imports.cue is evaluated as literal CUE data only -- no
// context.*/parameter.* expressions.
func parseCrateImports(data string) ([]CrateImport, error) {
	if err := nesting.Check(data, nesting.MaxDepth); err != nil {
		return nil, fmt.Errorf("_imports.cue: %w", err)
	}
	val := cuecontext.New().CompileString(data)
	if err := val.Err(); err != nil {
		return nil, fmt.Errorf("compile crates/_imports.cue: %w", err)
	}
	var raw rawCrateImports
	if err := val.Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode crates/_imports.cue: %w", err)
	}

	imports := make([]CrateImport, 0, len(raw.Imports))
	for _, imp := range raw.Imports {
		if imp.Crate == "" {
			return nil, fmt.Errorf("crates/_imports.cue: import missing required field \"crate\"")
		}
		if len(imp.Sources) != 1 {
			return nil, fmt.Errorf(
				"crates/_imports.cue: crate %q must declare exactly one sources[] entry (found %d); multi-source imports are not supported yet",
				imp.Crate, len(imp.Sources))
		}
		enabled := true
		if imp.Enabled != nil {
			enabled = *imp.Enabled
		}
		src := imp.Sources[0]
		if len(src.Versions) > 0 {
			klog.Warningf(
				"crates/_imports.cue: crate %q source declares versions %v, but the API-line filter is not enforced yet; every enabled line in the fetched crate will install",
				imp.Crate, src.Versions)
		}
		place, err := importPlacement(imp.Crate, imp.Placement)
		if err != nil {
			return nil, err
		}
		imports = append(imports, CrateImport{
			Crate:     imp.Crate,
			Enabled:   enabled,
			Registry:  src.Registry,
			Version:   src.Version,
			Placement: place,
		})
	}
	return imports, nil
}

// importPlacement checks one entry's placement field. fromPolicy cannot be
// mixed with a literal field, and a literal placement must pass the same check
// the crate provider applies to the component property, so a bad entry fails
// the addon load instead of the later crate render.
func importPlacement(crate string, raw *rawPlacement) (*ImportPlacement, error) {
	if raw == nil {
		return nil, nil
	}
	if raw.FromPolicy != "" {
		if raw.Clusters != nil || raw.ClusterLabelSelector != nil || raw.AllowEmpty != nil {
			return nil, fmt.Errorf("crates/_imports.cue: crate %q: placement.fromPolicy cannot be combined with clusters, clusterLabelSelector or allowEmpty", crate)
		}
		return &ImportPlacement{FromPolicy: raw.FromPolicy}, nil
	}
	place := &ImportPlacement{Clusters: raw.Clusters, ClusterLabelSelector: raw.ClusterLabelSelector}
	if raw.AllowEmpty != nil {
		place.AllowEmpty = *raw.AllowEmpty
	}
	if err := placement.Validate(place.policyPlacement()); err != nil {
		return nil, fmt.Errorf("crates/_imports.cue: crate %q: %w", crate, err)
	}
	return place, nil
}

// policyPlacement is p's literal fields as the placement type the crate
// provider checks.
func (p *ImportPlacement) policyPlacement() *v1alpha1.Placement {
	return &v1alpha1.Placement{Clusters: p.Clusters, ClusterLabelSelector: p.ClusterLabelSelector, AllowEmpty: p.AllowEmpty}
}

// readCrateImportsFile reads crates/_imports.cue (when present) and parses
// it into InstallPackage.Imports.
func readCrateImportsFile(a *InstallPackage, reader AsyncReader, readPath string) error {
	data, err := reader.ReadFile(readPath)
	if err != nil {
		return err
	}
	imports, err := parseCrateImports(data)
	if err != nil {
		return err
	}
	a.Imports = imports
	return nil
}

func uniqueImportedComponentName(base string, used map[string]bool) string {
	if !used[base] {
		return base
	}
	for i := 2; ; i++ {
		candidate := base + "-" + strconv.Itoa(i)
		if !used[candidate] {
			return candidate
		}
	}
}

// RenderCrateComponents builds one type: crate ApplicationComponent per
// enabled crates/_imports.cue entry. A crate name claimed anywhere else in
// the addon too -- a hand-written type: crate component in template.cue, or
// an inline crates/<name>/ -- fails the whole render; see
// checkCrateNameCollisions. Each emitted component depends on every name in
// resourceComponentNames, so the crate's resources never apply
// before the addon's own operators and CRDs are healthy. policies are the
// rendered addon Application's own policies, which an import's
// placement.fromPolicy names.
func RenderCrateComponents(addon *InstallPackage, existingComponents []common2.ApplicationComponent, resourceComponentNames []string, policies []v1beta1.AppPolicy) ([]common2.ApplicationComponent, error) {
	declaredBy := crateDeclaredBy(existingComponents, addon.Imports, addon.InlineCrates)
	if err := checkCrateNameCollisions(declaredBy); err != nil {
		return nil, err
	}

	usedNames := make(map[string]bool, len(existingComponents))
	for _, c := range existingComponents {
		usedNames[c.Name] = true
	}

	var comps []common2.ApplicationComponent
	for _, imp := range addon.Imports {
		if !imp.Enabled {
			continue
		}
		properties := map[string]interface{}{"crate": imp.Crate}
		if imp.Registry != "" {
			properties["registry"] = imp.Registry
		}
		if imp.Version != "" {
			properties["version"] = imp.Version
		}
		if imp.Placement != nil {
			place, err := resolvePlacement(imp.Crate, imp.Placement, policies)
			if err != nil {
				return nil, err
			}
			properties["placement"] = placement.TopologyProperties(*place)
		}
		raw, err := json.Marshal(properties)
		if err != nil {
			return nil, fmt.Errorf("render crate component %q for addon %q: %w", imp.Crate, addon.Name, err)
		}
		componentName := uniqueImportedComponentName(imp.Crate, usedNames)
		comps = append(comps, common2.ApplicationComponent{
			Name:       componentName,
			Type:       "crate",
			Properties: &runtime.RawExtension{Raw: raw},
			DependsOn:  slices.Clone(resourceComponentNames),
		})
		usedNames[componentName] = true
	}
	return comps, nil
}

// resolvePlacement returns an import's placement. A literal placement is used
// as it is. fromPolicy copies the named topology policy's clusters,
// clusterLabelSelector and allowEmpty, never its namespace: the namespace of a
// crate's objects follows the installing Application, not the addon's policy.
func resolvePlacement(crate string, place *ImportPlacement, policies []v1beta1.AppPolicy) (*v1alpha1.Placement, error) {
	if place.FromPolicy == "" {
		return place.policyPlacement(), nil
	}
	idx := slices.IndexFunc(policies, func(p v1beta1.AppPolicy) bool { return p.Name == place.FromPolicy })
	if idx < 0 {
		return nil, fmt.Errorf("crates/_imports.cue: crate %q: placement.fromPolicy %q: no such policy", crate, place.FromPolicy)
	}
	pol := policies[idx]
	if pol.Type != v1alpha1.TopologyPolicyType {
		return nil, fmt.Errorf("crates/_imports.cue: crate %q: placement.fromPolicy %q is a %s policy, not topology", crate, place.FromPolicy, pol.Type)
	}
	var spec v1alpha1.TopologyPolicySpec
	if pol.Properties != nil && len(pol.Properties.Raw) > 0 {
		if err := json.Unmarshal(pol.Properties.Raw, &spec); err != nil {
			return nil, fmt.Errorf("crates/_imports.cue: crate %q: placement.fromPolicy %q: %w", crate, place.FromPolicy, err)
		}
	}
	// Read the selector the way the topology policy itself does, so the crate
	// goes where the addon goes: the deprecated clusterSelector spelling counts
	// only while the DeprecatedPolicySpec feature gate is on.
	selector := policy.GetClusterLabelSelectorInTopology(&spec)
	if spec.Clusters == nil && selector == nil && spec.DeprecatedClusterSelector != nil {
		return nil, fmt.Errorf("crates/_imports.cue: crate %q: placement.fromPolicy %q: the policy sets the deprecated clusterSelector, which is ignored unless the %s feature gate is on; use clusterLabelSelector", crate, place.FromPolicy, features.DeprecatedPolicySpec)
	}
	resolved := &v1alpha1.Placement{Clusters: spec.Clusters, ClusterLabelSelector: selector, AllowEmpty: spec.AllowEmpty}
	if err := placement.Validate(resolved); err != nil {
		return nil, fmt.Errorf("crates/_imports.cue: crate %q: placement.fromPolicy %q: %w", crate, place.FromPolicy, err)
	}
	return resolved, nil
}

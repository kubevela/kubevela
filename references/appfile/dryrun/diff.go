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

package dryrun

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/aryann/difflib"
	"github.com/ghodss/yaml"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/dsl/definition"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
	oamutil "github.com/oam-dev/kubevela/pkg/oam/util"
)

// NewLiveDiffOption creates a live-diff option
func NewLiveDiffOption(c client.Client, dm discoverymapper.DiscoveryMapper, pd *definition.PackageDiscover, as []oam.Object) *LiveDiffOption {
	return &LiveDiffOption{NewDryRunOption(c, dm, pd, as)}
}

// ManifestKind enums the kind of OAM objects
type ManifestKind string

// enum kinds of manifest objects
const (
	AppKind           ManifestKind = "Application"
	AppConfigCompKind ManifestKind = "AppConfigComponent"
	RawCompKind       ManifestKind = "Component"
	TraitKind         ManifestKind = "Trait"
)

// DiffEntry records diff info of OAM object
type DiffEntry struct {
	Name     string               `json:"name"`
	Kind     ManifestKind         `json:"kind"`
	DiffType DiffType             `json:"diffType,omitempty"`
	Diffs    []difflib.DiffRecord `json:"diffs,omitempty"`
	Subs     []*DiffEntry         `json:"subs,omitempty"`
}

// DiffType enums the type of diff
type DiffType string

// enum types of diff
const (
	AddDiff    DiffType = "ADD"
	ModifyDiff DiffType = "MODIFY"
	RemoveDiff DiffType = "REMOVE"
	NoDiff     DiffType = ""
)

// manifest is a helper struct used to calculate diff on applications and
// sub-resources.
type manifest struct {
	Name string
	Kind ManifestKind
	// Data is unmarshalled object in YAML
	Data string
	// application's subs means appConfigComponents
	// appConfigComponent's subs means rawComponent and traits
	Subs []*manifest
}

// LiveDiffOption contains options for comparing an application with a
// living AppRevision in the cluster
type LiveDiffOption struct {
	DryRun
}

// Diff does three phases, dry-run on input app, preparing manifest for diff, and
// calculating diff on manifests.
func (l *LiveDiffOption) Diff(ctx context.Context, app *v1beta1.Application, appRevision *v1beta1.ApplicationRevision) (*DiffEntry, error) {
	ac, comps, err := l.ExecuteDryRun(ctx, app)
	if err != nil {
		return nil, errors.WithMessagef(err, "cannot dry-run for app %q", app.Name)
	}
	// new refers to the app as input to dry-run
	newManifest, err := generateManifest(app, ac, comps)
	if err != nil {
		return nil, errors.WithMessagef(err, "cannot generate diff manifest for app %q", app.Name)
	}

	// old refers to the living app revision
	oldManifest, err := generateManifestFromAppRevision(appRevision)
	if err != nil {
		return nil, errors.WithMessagef(err, "cannot generate diff manifest for AppRevision %q", appRevision.Name)
	}
	diffResult := l.calculateDiff(oldManifest, newManifest)
	return diffResult, nil
}

// calculateDiff calculate diff between two application and their sub-resources
func (l *LiveDiffOption) calculateDiff(oldApp, newApp *manifest) *DiffEntry {
	emptyManifest := &manifest{}
	r := &DiffEntry{
		Name: oldApp.Name,
		Kind: oldApp.Kind,
	}

	appDiffs := diffManifest(oldApp, newApp)
	if hasChanges(appDiffs) {
		r.DiffType = ModifyDiff
		r.Diffs = appDiffs
	}

	// check modified and removed components
	for _, oldAcc := range oldApp.Subs {
		accDiffEntry := &DiffEntry{
			Name: oldAcc.Name,
			Kind: oldAcc.Kind,
			Subs: make([]*DiffEntry, 0),
		}

		var newAcc *manifest
		// check whether component is removed
		for _, acc := range newApp.Subs {
			if oldAcc.Name == acc.Name {
				newAcc = acc
				break
			}
		}
		if newAcc != nil {
			// component is not removed
			// check modified and removed ACC subs (rawComponent and traits)
			for _, oldAccSub := range oldAcc.Subs {
				accSubDiffEntry := &DiffEntry{
					Name: oldAccSub.Name,
					Kind: oldAccSub.Kind,
				}
				var newAccSub *manifest
				for _, accSub := range newAcc.Subs {
					if accSub.Kind == oldAccSub.Kind &&
						accSub.Name == oldAccSub.Name {
						newAccSub = accSub
						break
					}
				}
				var diffs []difflib.DiffRecord
				if newAccSub != nil {
					// accSub is not removed, then check modification
					diffs = diffManifest(oldAccSub, newAccSub)
					if hasChanges(diffs) {
						accSubDiffEntry.DiffType = ModifyDiff
					} else {
						accSubDiffEntry.DiffType = NoDiff
					}
				} else {
					// accSub is removed
					diffs = diffManifest(oldAccSub, emptyManifest)
					accSubDiffEntry.DiffType = RemoveDiff
				}
				accSubDiffEntry.Diffs = diffs
				accDiffEntry.Subs = append(accDiffEntry.Subs, accSubDiffEntry)
			}

			// check added ACC subs (traits)
			for _, newAccSub := range newAcc.Subs {
				isAdded := true
				for _, oldAccSub := range oldAcc.Subs {
					if oldAccSub.Kind == newAccSub.Kind &&
						oldAccSub.Name == newAccSub.Name {
						isAdded = false
						break
					}
				}
				if isAdded {
					accSubDiffEntry := &DiffEntry{
						Name:     newAccSub.Name,
						Kind:     newAccSub.Kind,
						DiffType: AddDiff,
					}
					diffs := diffManifest(emptyManifest, newAccSub)
					accSubDiffEntry.Diffs = diffs
					accDiffEntry.Subs = append(accDiffEntry.Subs, accSubDiffEntry)
				}
			}
		} else {
			// component is removed as well as its subs
			accDiffEntry.DiffType = RemoveDiff
			for _, oldAccSub := range oldAcc.Subs {
				diffs := diffManifest(oldAccSub, emptyManifest)
				accSubDiffEntry := &DiffEntry{
					Name:     oldAccSub.Name,
					Kind:     oldAccSub.Kind,
					DiffType: RemoveDiff,
					Diffs:    diffs,
				}
				accDiffEntry.Subs = append(accDiffEntry.Subs, accSubDiffEntry)
			}
		}
		r.Subs = append(r.Subs, accDiffEntry)
	}

	// check added component
	for _, newAcc := range newApp.Subs {
		isAdded := true
		for _, oldAcc := range oldApp.Subs {
			if oldAcc.Kind == newAcc.Kind &&
				oldAcc.Name == newAcc.Name {
				isAdded = false
				break
			}
		}
		if isAdded {
			accDiffEntry := &DiffEntry{
				Name:     newAcc.Name,
				Kind:     newAcc.Kind,
				DiffType: AddDiff,
				Subs:     make([]*DiffEntry, 0),
			}
			// added component's subs are all added
			for _, newAccSub := range newAcc.Subs {
				diffs := diffManifest(emptyManifest, newAccSub)
				accSubDiffEntry := &DiffEntry{
					Name:     newAccSub.Name,
					Kind:     newAccSub.Kind,
					DiffType: AddDiff,
					Diffs:    diffs,
				}
				accDiffEntry.Subs = append(accDiffEntry.Subs, accSubDiffEntry)
			}
			r.Subs = append(r.Subs, accDiffEntry)
		}
	}

	return r
}

// generateManifest generates a manifest whose top-level is an application
func generateManifest(app *v1beta1.Application, ac *v1alpha2.ApplicationConfiguration, comps []*v1alpha2.Component) (*manifest, error) {
	r := &manifest{
		Name: app.Name,
		Kind: AppKind,
	}
	b, err := yaml.Marshal(app)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot marshal application %q", app.Name)
	}
	r.Data = string(b)
	appSubs := make([]*manifest, 0, len(app.Spec.Components))

	// a helper map recording all rawComponents with compName as key
	rawCompManifests := map[string]*manifest{}
	for _, comp := range comps {
		cM := &manifest{
			Name: comp.Name,
			Kind: RawCompKind,
		}
		// dry-run doesn't set namespace and ownerRef to a component
		// we should remove them before comparing
		comp.SetNamespace("")
		comp.SetOwnerReferences(nil)
		if err := emptifyAppRevisionLabel(&comp.Spec.Workload); err != nil {
			return nil, errors.WithMessagef(err, "cannot emptify appRevision label in component %q", comp.Name)
		}
		b, err := yaml.Marshal(comp)
		if err != nil {
			return nil, errors.Wrapf(err, "cannot marshal component %q", comp.Name)
		}
		cM.Data = string(b)
		rawCompManifests[comp.Name] = cM
	}

	// generate appConfigComponent manifests
	for _, acc := range ac.Spec.Components {
		if acc.ComponentName == "" && acc.RevisionName != "" {
			// dry-run cannot generate revision name
			// we should compare with comp name rather than revision name
			acc.ComponentName = extractNameFromRevisionName(acc.RevisionName)
			acc.RevisionName = ""
		}

		accM := &manifest{
			Name: acc.ComponentName,
			Kind: AppConfigCompKind,
		}
		// get matched raw component and add it into appConfigComponent's subs
		subs := []*manifest{rawCompManifests[acc.ComponentName]}
		for _, t := range acc.Traits {
			if err := emptifyAppRevisionLabel(&t.Trait); err != nil {
				return nil, errors.WithMessage(err, "cannot emptify appRevision label of trait")
			}
			tObj, err := oamutil.RawExtension2Unstructured(&t.Trait)
			if err != nil {
				return nil, errors.WithMessage(err, "cannot parser trait raw")
			}

			tType := tObj.GetLabels()[oam.TraitTypeLabel]
			tResource := tObj.GetLabels()[oam.TraitResource]
			// dry-run cannot generate name for a trait
			// a join of trait tyupe&resource is unique in a component
			// we use it to identify a trait
			tUnique := fmt.Sprintf("%s/%s", tType, tResource)

			b, err := yaml.JSONToYAML(t.Trait.Raw)
			if err != nil {
				return nil, errors.Wrapf(err, "cannot parse trait %q raw to YAML", tUnique)
			}
			subs = append(subs, &manifest{
				Name: tUnique,
				Kind: TraitKind,
				Data: string(b),
			})
		}
		accM.Subs = subs
		appSubs = append(appSubs, accM)
	}
	r.Subs = appSubs
	return r, nil
}

// generateManifestFromAppRevision generates manifest from an AppRevision
func generateManifestFromAppRevision(appRevision *v1beta1.ApplicationRevision) (*manifest, error) {
	ac := &v1alpha2.ApplicationConfiguration{}
	if err := json.Unmarshal(appRevision.Spec.ApplicationConfiguration.Raw, ac); err != nil {
		return nil, errors.Wrap(err, "cannot unmarshal appconfig")
	}

	comps := []*v1alpha2.Component{}
	for _, rawComp := range appRevision.Spec.Components {
		c := &v1alpha2.Component{}
		if err := json.Unmarshal(rawComp.Raw.Raw, c); err != nil {
			return nil, errors.Wrap(err, "cannot unmarshal component")
		}
		comps = append(comps, c)
	}

	app := appRevision.Spec.Application
	// app in appRevision has no name & namespace
	// we should extract/get them from appRappRevision
	app.Name = extractNameFromRevisionName(appRevision.Name)
	app.Namespace = appRevision.Namespace
	return generateManifest(&app, ac, comps)
}

// diffManifest calculates diff between data of two manifest line by line
func diffManifest(old, new *manifest) []difflib.DiffRecord {
	const sep = "\n"
	return difflib.Diff(strings.Split(old.Data, sep), strings.Split(new.Data, sep))
}

func extractNameFromRevisionName(r string) string {
	s := strings.Split(r, "-")
	return strings.Join(s[0:len(s)-1], "-")
}

// emptifyAppRevisionLabel will set label oam.LabelAppRevision to empty
// because dry-run cannot set value to this lable
func emptifyAppRevisionLabel(o *runtime.RawExtension) error {
	u, err := oamutil.RawExtension2Unstructured(o)
	if err != nil {
		return errors.WithMessage(err, "cannot reset appRevision label of raw object")
	}
	newLabels := map[string]string{}
	labels := u.GetLabels()
	for k, v := range labels {
		if k == oam.LabelAppRevision {
			newLabels[k] = ""
			continue
		}
		newLabels[k] = v
	}
	u.SetLabels(newLabels)
	b, err := u.MarshalJSON()
	if err != nil {
		return errors.WithMessage(err, "cannot reset appRevision label of raw object")
	}
	o.Raw = b
	return nil
}

// hasChanges checks whether existing change in diff records
func hasChanges(diffs []difflib.DiffRecord) bool {
	for _, d := range diffs {
		// diffliib.Common means no change between two sides
		if d.Delta != difflib.Common {
			return true
		}
	}
	return false
}

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
	"fmt"
	"strings"

	"github.com/aryann/difflib"
	"github.com/pkg/errors"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/appfile"
	"github.com/oam-dev/kubevela/pkg/cue/packages"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
)

// NewLiveDiffOption creates a live-diff option
func NewLiveDiffOption(c client.Client, cfg *rest.Config, dm discoverymapper.DiscoveryMapper, pd *packages.PackageDiscover, as []oam.Object) *LiveDiffOption {
	parser := appfile.NewApplicationParser(c, dm, pd)
	return &LiveDiffOption{DryRun: NewDryRunOption(c, cfg, dm, pd, as), Parser: parser}
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
	Parser *appfile.Parser
}

// Diff does three phases, dry-run on input app, preparing manifest for diff, and
// calculating diff on manifests.
func (l *LiveDiffOption) Diff(ctx context.Context, app *v1beta1.Application, appRevision *v1beta1.ApplicationRevision) (*DiffEntry, error) {
	comps, err := l.ExecuteDryRun(ctx, app)
	if err != nil {
		return nil, errors.WithMessagef(err, "cannot dry-run for app %q", app.Name)
	}
	// new refers to the app as input to dry-run
	newManifest, err := generateManifest(app, comps)
	if err != nil {
		return nil, errors.WithMessagef(err, "cannot generate diff manifest for app %q", app.Name)
	}

	// old refers to the living app revision
	oldManifest, err := generateManifestFromAppRevision(l.Parser, appRevision)
	if err != nil {
		return nil, errors.WithMessagef(err, "cannot generate diff manifest for AppRevision %q", appRevision.Name)
	}
	diffResult := l.calculateDiff(oldManifest, newManifest)
	return diffResult, nil
}

// DiffApps does three phases, dry-run on input app, preparing manifest for diff, and
// calculating diff on manifests.
func (l *LiveDiffOption) DiffApps(ctx context.Context, app *v1beta1.Application, oldApp *v1beta1.Application) (*DiffEntry, error) {
	comps, err := l.ExecuteDryRun(ctx, app)
	if err != nil {
		return nil, errors.WithMessagef(err, "cannot dry-run for app %q", app.Name)
	}
	// new refers to the app as input to dry-run
	newManifest, err := generateManifest(app, comps)
	if err != nil {
		return nil, errors.WithMessagef(err, "cannot generate diff manifest for app %q", app.Name)
	}

	oldComps, err := l.ExecuteDryRun(ctx, oldApp)
	if err != nil {
		return nil, errors.WithMessagef(err, "cannot dry-run for app %q", oldApp.Name)
	}
	// new refers to the app as input to dry-run
	oldManifest, err := generateManifest(oldApp, oldComps)
	if err != nil {
		return nil, errors.WithMessagef(err, "cannot generate diff manifest for app %q", oldApp.Name)
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
func generateManifest(app *v1beta1.Application, comps []*types.ComponentManifest) (*manifest, error) {
	r := &manifest{
		Name: app.Name,
		Kind: AppKind,
	}
	removeRevisionRelatedLabelAndAnnotation(app)
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
		removeRevisionRelatedLabelAndAnnotation(comp.StandardWorkload)
		b, err := yaml.Marshal(comp.StandardWorkload)
		if err != nil {
			return nil, errors.Wrapf(err, "cannot marshal component %q", comp.Name)
		}
		cM.Data = string(b)
		rawCompManifests[comp.Name] = cM
	}

	// generate appConfigComponent manifests
	for _, comp := range comps {
		compM := &manifest{
			Name: comp.Name,
			Kind: AppConfigCompKind,
		}
		comp.RevisionHash = ""
		comp.RevisionName = ""
		// get matched raw component and add it into appConfigComponent's subs
		subs := []*manifest{rawCompManifests[comp.Name]}
		for _, t := range comp.Traits {
			removeRevisionRelatedLabelAndAnnotation(t)

			tType := t.GetLabels()[oam.TraitTypeLabel]
			tResource := t.GetLabels()[oam.TraitResource]
			// dry-run cannot generate name for a trait
			// a join of trait type&resource is unique in a component
			// we use it to identify a trait
			tUnique := fmt.Sprintf("%s/%s", tType, tResource)

			b, err := yaml.Marshal(t)
			if err != nil {
				return nil, errors.Wrapf(err, "cannot parse trait %q raw to YAML", tUnique)
			}
			subs = append(subs, &manifest{
				Name: tUnique,
				Kind: TraitKind,
				Data: string(b),
			})
		}
		compM.Subs = subs
		appSubs = append(appSubs, compM)
	}
	r.Subs = appSubs
	return r, nil
}

// generateManifestFromAppRevision generates manifest from an AppRevision
func generateManifestFromAppRevision(parser *appfile.Parser, appRevision *v1beta1.ApplicationRevision) (*manifest, error) {
	af, err := parser.GenerateAppFileFromRevision(appRevision)
	if err != nil {
		return nil, err
	}
	comps, err := af.GenerateComponentManifests()
	if err != nil {
		return nil, err
	}
	app := appRevision.Spec.Application
	// app in appRevision has no name & namespace
	// we should extract/get them from appRappRevision
	app.Name = extractNameFromRevisionName(appRevision.Name)
	app.Namespace = appRevision.Namespace
	return generateManifest(&app, comps)
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

// removeRevisionRelatedLabelAndAnnotation will set label oam.LabelAppRevision to empty
// because dry-run cannot set value to this label
func removeRevisionRelatedLabelAndAnnotation(o client.Object) {
	newLabels := map[string]string{}
	labels := o.GetLabels()
	for k, v := range labels {
		if k == oam.LabelAppRevision {
			newLabels[k] = ""
			continue
		}
		newLabels[k] = v
	}
	o.SetLabels(newLabels)

	newAnnotations := map[string]string{}
	annotations := o.GetAnnotations()
	for k, v := range annotations {
		if k == oam.AnnotationKubeVelaVersion || k == oam.AnnotationAppRevision || k == "kubectl.kubernetes.io/last-applied-configuration" {
			continue
		}
		newAnnotations[k] = v
	}
	o.SetAnnotations(newAnnotations)
}

// hasChanges checks whether existing change in diff records
func hasChanges(diffs []difflib.DiffRecord) bool {
	for _, d := range diffs {
		// difflib.Common means no change between two sides
		if d.Delta != difflib.Common {
			return true
		}
	}
	return false
}

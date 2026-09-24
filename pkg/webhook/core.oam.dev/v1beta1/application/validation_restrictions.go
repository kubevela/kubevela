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
	velacache "github.com/oam-dev/kubevela/pkg/cache"
	"github.com/oam-dev/kubevela/pkg/definition/nsrestrict"
	"github.com/oam-dev/kubevela/pkg/features"
	"github.com/oam-dev/kubevela/pkg/oam"
	oamutil "github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/pkg/workflow/step"
)

// ValidateDefinitionRestrictions rejects an Application that uses a definition
// restricted to other namespaces, or that would put its namespace over a quota.
// The warnings it returns are quotas asking to be flagged before they refuse.
//
// Admission is the only place either is enforced, on update as well as create, so
// an Application admitted before a restriction keeps reconciling but is frozen.
//
// Costs a Get per definition type, and a namespace List per type a quota governs.
func (h *ValidatingHandler) ValidateDefinitionRestrictions(ctx context.Context, app, oldApp *v1beta1.Application) (field.ErrorList, []string) {
	if !utilfeature.DefaultMutableFeatureGate.Enabled(features.RestrictDefinitionNamespaces) {
		return nil, nil
	}

	c := &restrictionCheck{
		h:      h,
		ctx:    ctx,
		defCtx: oamutil.SetNamespaceInCtx(ctx, app.Namespace),
		app:    app,
		oldApp: oldApp,
	}
	usage := collectDefinitionUsage(app)

	c.checkComponents(usage.componentTypes)
	c.checkTraits(usage.traitTypes)
	for defType, indices := range usage.policyTypes {
		c.checkDefinition(defType, func() client.Object { return &v1beta1.PolicyDefinition{} },
			policyTypePaths(indices))
	}
	for defType, locations := range usage.workflowStepTypes {
		// A builtin step has no definition to restrict.
		if step.IsBuiltinWorkflowStepType(defType) {
			continue
		}
		c.checkDefinition(defType, func() client.Object { return &v1beta1.WorkflowStepDefinition{} },
			workflowStepTypePaths(locations))
	}
	for defType, indices := range usage.sourceTypes {
		c.checkDefinition(defType, func() client.Object { return &v1beta1.SourceDefinition{} },
			sourceTypePaths(indices))
	}

	return c.errs, c.warnings
}

// restrictionCheck holds what one request's checks share: the namespace labels,
// read at most once, and the errors and warnings they accumulate.
type restrictionCheck struct {
	h      *ValidatingHandler
	ctx    context.Context
	defCtx context.Context
	app    *v1beta1.Application
	// oldApp is what this Application looked like before, or nil on create.
	oldApp *v1beta1.Application

	errs     field.ErrorList
	warnings []string

	nsLabels      map[string]string
	nsAnnotations map[string]string
	nsErr         error
	nsLoaded      bool
}

// checkComponents enforces the restriction on each component type, then the quota.
func (c *restrictionCheck) checkComponents(componentTypes map[string][]int) {
	byName := newUsageGroup()
	for defType, indices := range componentTypes {
		used := componentTypePaths(indices)
		def := c.checkDefinition(defType, func() client.Object { return &v1beta1.ComponentDefinition{} }, used)
		byName.add(def, baseDefinitionName(defType), used)
	}
	byName.each(func(def client.Object, name string, paths []*field.Path) {
		c.checkQuota(def, velacache.UsageComponent, name, paths)
	})
}

// checkTraits does the same for traits, which carry their own budget: a component
// type and a trait type may share a name without sharing a quota.
func (c *restrictionCheck) checkTraits(traitTypes map[string][][2]int) {
	byName := newUsageGroup()
	for defType, locations := range traitTypes {
		used := traitTypePaths(locations)
		def := c.checkDefinition(defType, func() client.Object { return &v1beta1.TraitDefinition{} }, used)
		byName.add(def, baseDefinitionName(defType), used)
	}
	byName.each(func(def client.Object, name string, paths []*field.Path) {
		c.checkQuota(def, velacache.UsageTrait, name, paths)
	})
}

// usageGroup gathers the places one definition was used. webservice and
// webservice@v1 name one definition, so they share a budget: one count, one
// warning, and a refusal at every path that contributed.
type usageGroup struct {
	defs  map[string]client.Object
	paths map[string][]*field.Path
	order []string
}

func newUsageGroup() *usageGroup {
	return &usageGroup{defs: map[string]client.Object{}, paths: map[string][]*field.Path{}}
}

// add ignores a nil definition, which is one unreadable or already refused.
func (g *usageGroup) add(def client.Object, name string, paths []*field.Path) {
	if def == nil {
		return
	}
	if _, seen := g.defs[name]; !seen {
		g.order = append(g.order, name)
	}
	g.defs[name] = def
	g.paths[name] = append(g.paths[name], paths...)
}

func (g *usageGroup) each(fn func(def client.Object, name string, paths []*field.Path)) {
	for _, name := range g.order {
		fn(g.defs[name], name, g.paths[name])
	}
}

// checkDefinition enforces the namespace restriction and hands the definition back,
// so a caller that needs more from it does not fetch it twice. It returns nil when
// the definition is unreadable or already refused.
func (c *restrictionCheck) checkDefinition(defType string, newDef func() client.Object, paths []*field.Path) client.Object {
	// Read the definition itself, never a DefinitionRevision: webservice@v1 names
	// the definition "webservice" rendered from a frozen revision, so strip the pin
	// and the current restriction applies, whatever revision is pinned.
	//
	// The cached client, so a restriction written moments ago may not be visible.
	// Definitions are cached for the controller already, which makes this free; an
	// uncached read would cost an API call per type on every admission.
	name := baseDefinitionName(defType)
	def := newDef()
	if err := oamutil.GetDefinition(c.defCtx, c.h.Client, def, name); err != nil {
		if errors.IsNotFound(err) {
			// ValidateComponents reports a missing definition, and better.
			return nil
		}
		// Anything else leaves the restrictions unread, and an unread restriction
		// is not an absent one. Refuse, as a retryable failure.
		klog.Errorf("Failed to load %s %q to check its restrictions: %v", nsrestrict.KindOf(def), name, err)
		c.unevaluable(paths, fmt.Errorf("cannot read %s %q to check its restrictions: %w",
			nsrestrict.KindOf(def), name, err))
		return nil
	}

	var labels map[string]string
	if nsrestrict.NeedsNamespaceLabels(def, c.app.Namespace) {
		var err error
		if labels, err = c.namespaceLabels(); err != nil {
			c.unevaluable(paths, fmt.Errorf(
				"cannot evaluate the namespace restriction on %s %q: reading namespace %q: %w",
				nsrestrict.KindOf(def), name, c.app.Namespace, err))
			return nil
		}
	}

	if err := nsrestrict.Check(def, c.app.Namespace, labels); err != nil {
		// The operator gets the patterns, through the log. The Application's author
		// gets only that the definition is restricted.
		klog.Infof("Denied %s %q to Application %q in namespace %q: allowed for %s",
			nsrestrict.KindOf(def), name, c.app.Name, c.app.Namespace, nsrestrict.Describe(def))
		c.forbid(paths, err.Error())
		return nil
	}
	return def
}

// checkQuota counts the namespace's uses of a definition against its quota. name is
// the definition's, pin already stripped, and kind says what is being counted.
func (c *restrictionCheck) checkQuota(def client.Object, kind, name string, paths []*field.Path) {
	var labels map[string]string
	if nsrestrict.QuotaNeedsNamespaceLabels(def, c.app.Namespace) {
		var err error
		if labels, err = c.namespaceLabels(); err != nil {
			c.unevaluable(paths, fmt.Errorf(
				"cannot evaluate the quota on %s %q: reading namespace %q: %w",
				nsrestrict.KindOf(def), name, c.app.Namespace, err))
			return
		}
	}
	quota := nsrestrict.QuotaFor(def, c.app.Namespace, labels)
	if quota == nil {
		return
	}

	// Checked once a quota is known to apply, so a definition without one still
	// costs no namespace read, and before the count, so an exempt namespace costs
	// no list either. Logged because the opt-out is meant to be temporary, and one
	// left in place should be findable.
	// An unreadable namespace leaves the quota in force rather than refusing the
	// Application: the opt-out only ever relaxes, so losing it errs towards the
	// limit holding, which is the safe direction and costs nobody an outage.
	annotations, _ := c.namespaceAnnotations()
	if nsrestrict.QuotaExempt(annotations) {
		klog.Infof("Namespace %q is annotated %s=true, so the quota on %s %q is not applied to Application %q",
			c.app.Namespace, oam.AnnotationQuotaExempt, nsrestrict.KindOf(def), name, c.app.Name)
		return
	}

	reader, indexed := c.h.quotaReader()
	existing, countErr := countUsage(c.ctx, reader, indexed, c.app.Namespace, kind, name,
		c.app.Name) // excluding itself, or an edit fails its own quota
	if countErr != nil {
		klog.Errorf("Failed to count uses of %q in namespace %q for its quota: %v", name, c.app.Namespace, countErr)
		c.unevaluable(paths, fmt.Errorf(
			"cannot evaluate the quota on %s %q: counting its use in namespace %q: %w",
			nsrestrict.KindOf(def), name, c.app.Namespace, countErr))
		return
	}
	incoming := usageInApp(c.app, kind, name)
	total := existing + incoming

	// No transaction, so concurrent creates can each pass a limit they jointly
	// breach. This blocks a request; it does not guarantee the ceiling.
	refuse, warn := nsrestrict.ExceedsQuota(quota, total)

	// A namespace already over its quota, because the quota was lowered under it,
	// still has to be drainable. An edit that does not add to this Application's
	// own use is admitted and flagged, or the only way back under a new limit
	// would be deleting whole Applications.
	if refuse && c.oldApp != nil && incoming <= usageInApp(c.oldApp, kind, name) {
		c.warnings = append(c.warnings, fmt.Sprintf(
			"%s %q: namespace %q is over its quota at %d of the %d allowed, and this change does not add to it.",
			nsrestrict.KindOf(def), name, c.app.Namespace, total, *quota.Limit))
		return
	}

	switch {
	case refuse:
		// The author gets the namespace and the type, which is what they can act
		// on. The counts go to the operator, as the restriction patterns do.
		// ExceedsQuota refuses only against a ceiling, so Limit is set.
		klog.Infof("Denied %s %q to Application %q: namespace %q allows %d and this would make %d",
			nsrestrict.KindOf(def), name, c.app.Name, c.app.Namespace, *quota.Limit, total)
		c.forbid(paths, fmt.Sprintf("this would exceed the quota for %s type %q in namespace %q",
			kind, name, c.app.Namespace))
	case warn:
		c.warnings = append(c.warnings,
			quotaWarning(nsrestrict.KindOf(def), name, c.app.Namespace, total, quota.Limit))
	}
}

// namespaceLabels reads the Application's namespace, once per request. A
// restriction by name never asks for it, and so costs no extra API call.
func (c *restrictionCheck) namespaceLabels() (map[string]string, error) {
	if err := c.loadNamespace(); err != nil {
		return nil, err
	}
	return c.nsLabels, nil
}

// namespaceAnnotations reads the same Namespace, for the quota opt-out.
func (c *restrictionCheck) namespaceAnnotations() (map[string]string, error) {
	if err := c.loadNamespace(); err != nil {
		return nil, err
	}
	return c.nsAnnotations, nil
}

func (c *restrictionCheck) loadNamespace() error {
	if c.nsLoaded {
		return c.nsErr
	}
	c.nsLoaded = true
	ns := &corev1.Namespace{}
	if err := c.h.namespaceReader().Get(c.ctx, apitypes.NamespacedName{Name: c.app.Namespace}, ns); err != nil {
		klog.Errorf("Failed to read namespace %q to check a definition's restrictions: %v", c.app.Namespace, err)
		c.nsErr = err
		return err
	}
	// An empty map still matches a selector that requires nothing; nil, returned
	// above on error, matches none.
	c.nsLabels, c.nsAnnotations = ns.Labels, ns.Annotations
	if c.nsLabels == nil {
		c.nsLabels = map[string]string{}
	}
	return nil
}

// forbid records a policy violation at every path the definition was used at.
func (c *restrictionCheck) forbid(paths []*field.Path, detail string) {
	for _, p := range paths {
		c.errs = append(c.errs, field.Forbidden(p, detail))
	}
}

// unevaluable refuses the Application for a server-side failure, which is worth a
// retry rather than a policy violation to fix.
func (c *restrictionCheck) unevaluable(paths []*field.Path, err error) {
	for _, p := range paths {
		c.errs = append(c.errs, field.InternalError(p, err))
	}
}

// quotaWarning phrases how close a namespace is to a quota. With no limit there is
// no ceiling to name.
func quotaWarning(kind, name, namespace string, total int, limit *int32) string {
	if limit != nil {
		return fmt.Sprintf("%s %q: namespace %q is using %d of the %d allowed.",
			kind, name, namespace, total, *limit)
	}
	return fmt.Sprintf("%s %q: namespace %q is using %d, at or above the level this definition asks to be flagged at.",
		kind, name, namespace, total)
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

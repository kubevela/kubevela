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

package mcp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
)

const (
	// diffTokenTTL is how long a diff token stays valid. Long enough for a
	// human to read the change set, short enough that the cluster it was
	// computed against is still the cluster it is applied to.
	diffTokenTTL = 15 * time.Minute
	// maxDiffTokens bounds the in-memory token cache.
	maxDiffTokens = 256
	// baseNew marks a diff against no existing revision.
	baseNew = "new"
)

// DiffInput is the Application to compare with what runs.
type DiffInput struct {
	PlanInput
	Revision string `json:"revision,omitempty" jsonschema:"ApplicationRevision to compare against. Empty uses the latest revision; a missing Application compares against nothing."`
}

// DiffOutput is the structured result of diff_application.
type DiffOutput struct {
	Application string `json:"application"`
	Namespace   string `json:"namespace"`
	// Base names the ApplicationRevision the target was compared with, or
	// "new" when the Application does not exist.
	Base      string    `json:"base" jsonschema:"ApplicationRevision the document was compared with, or new when the Application does not exist."`
	ChangeSet ChangeSet `json:"changeSet" jsonschema:"Everything that would change, computed from both renders."`
	Risk      Risk      `json:"risk" jsonschema:"Risk class with the rules that fired. Show it to the user with the change set."`
	// Token binds this change set to the base revision and the submitted
	// document. apply_application requires it, so nothing is applied that
	// was not shown first.
	Token     string    `json:"token" jsonschema:"Required by apply_application. Binds this exact document to the base revision; it does not record that anyone reviewed the diff."`
	ExpiresAt time.Time `json:"expiresAt" jsonschema:"When the token stops being accepted, UTC."`
	Evidence  []string  `json:"evidence" jsonschema:"Objects and renders this answer was derived from."`
	Meta      Meta      `json:"meta"`
}

// diffRecord is what the token stands for. The apply path checks that the
// live revision still matches and that the document is byte-identical.
type diffRecord struct {
	appKey   client.ObjectKey
	base     string
	specHash string
	// liveExists and liveRV describe the live Application when the diff was
	// computed. apply_application writes only if both still hold.
	liveExists bool
	liveRV     string
	// liveUID tells a same-name replacement from the object that was
	// diffed; a recreated Application can match on everything else.
	liveUID string
	// liveGeneration and liveMeta capture the live Application's intent:
	// the spec generation and its user labels and annotations. Status
	// writes by the controller change the resourceVersion but not these.
	liveGeneration int64
	liveMeta       string
	changeSet      ChangeSet
	risk           Risk
	createdAt      time.Time
	// localDefinitions counts definitions the caller supplied to the diff.
	// The token does not bind them, so a diff that used any cannot drive a
	// direct write: its risk class came from definitions the cluster does
	// not have.
	localDefinitions int
	// targetNamespaces are the namespaces the target render writes into.
	targetNamespaces []string
	// opaqueSteps are workflow step types whose writes the render does not
	// show, such as apply-object or webhook steps.
	opaqueSteps []string
	// promptNonce is set when apply_application asks the user to confirm.
	// The answering call must echo it, so a confirmation is only accepted
	// for a prompt this server issued.
	promptNonce string
}

// setNonce records the prompt nonce on a live token.
func (c *diffCache) setNonce(token, nonce string) bool {
	rec, ok := c.lookup(token)
	if !ok {
		return false
	}
	rec.promptNonce = nonce
	c.entries[token] = rec
	return true
}

// take returns a live token's record and removes it, so a token is used
// once. A failed lookup leaves the cache unchanged.
func (c *diffCache) take(token string) (diffRecord, bool) {
	rec, ok := c.lookup(token)
	if ok {
		delete(c.entries, token)
	}
	return rec, ok
}

// diffCache holds issued tokens for diffTokenTTL.
type diffCache struct {
	entries map[string]diffRecord
}

func (c *diffCache) put(token string, rec diffRecord) {
	if c.entries == nil {
		c.entries = map[string]diffRecord{}
	}
	now := time.Now()
	for t, e := range c.entries {
		if now.Sub(e.createdAt) > diffTokenTTL {
			delete(c.entries, t)
		}
	}
	if len(c.entries) >= maxDiffTokens {
		var oldest string
		for t, e := range c.entries {
			if oldest == "" || e.createdAt.Before(c.entries[oldest].createdAt) {
				oldest = t
			}
		}
		delete(c.entries, oldest)
	}
	c.entries[token] = rec
}

// lookup returns the record for a live token, or false.
func (c *diffCache) lookup(token string) (diffRecord, bool) {
	rec, ok := c.entries[token]
	if !ok || time.Since(rec.createdAt) > diffTokenTTL {
		return diffRecord{}, false
	}
	return rec, true
}

func (d *deps) diffApplication(ctx context.Context, _ *sdk.CallToolRequest, in DiffInput) (*sdk.CallToolResult, DiffOutput, error) {
	var out DiffOutput
	app, defs, err := d.parsePlanInput(in.PlanInput)
	if err != nil {
		return nil, out, err
	}
	out.Application, out.Namespace = app.Name, app.Namespace
	cli := d.f.Client()
	if cli == nil {
		return nil, out, errNoConfig
	}

	baseApp, baseName, baseDefs, live, err := d.loadBase(ctx, cli, app, in.Revision)
	if err != nil {
		return nil, out, err
	}
	out.Base = baseName
	out.Evidence = []string{fmt.Sprintf("target rendered from the submitted document for %s/%s", app.Namespace, app.Name)}

	target, err := d.render(ctx, app, defs)
	if err != nil {
		return nil, out, err
	}
	var base *rendered
	if baseApp != nil {
		base, err = d.render(ctx, baseApp, baseDefs)
		if err != nil {
			return nil, out, fmt.Errorf("render base %s: %w", baseName, err)
		}
		switch {
		case len(baseDefs) > 0:
			out.Evidence = append(out.Evidence, fmt.Sprintf("base rendered from ApplicationRevision %s/%s with the definitions it recorded (%d)", app.Namespace, baseName, len(baseDefs)))
		case live != nil && live.Status.LatestRevision == nil && in.Revision == "":
			out.Evidence = append(out.Evidence, fmt.Sprintf("base rendered from the %s with the cluster's current definitions", baseName))
		default:
			out.Evidence = append(out.Evidence, fmt.Sprintf("base rendered from ApplicationRevision %s/%s with the cluster's current definitions, as it recorded none", app.Namespace, baseName))
		}
		for _, name := range driftedDefinitions(ctx, cli, baseDefs) {
			out.Meta.Warnings = append(out.Meta.Warnings, fmt.Sprintf("definition %s changed since %s; part of this change set comes from the definition, not the document", name, baseName))
		}
	} else {
		out.Evidence = append(out.Evidence, fmt.Sprintf("no Application %s/%s exists; compared against nothing", app.Namespace, app.Name))
	}

	out.ChangeSet = buildChangeSet(base, target)
	out.Risk = classify(out.ChangeSet)
	out.Token, out.ExpiresAt = d.issueToken(app, in.Application, baseName, live, out.ChangeSet, out.Risk, len(defs), renderedNamespaces(target))
	warnings := out.Meta.Warnings
	out.Meta = d.meta(ctx)
	out.Meta.Warnings = append(out.Meta.Warnings, warnings...)
	out.Meta.Source = SourceRendered
	return nil, out, nil
}

// loadBase returns the Application spec the target is compared with: the
// named revision, else the live Application's latest revision, else nil
// for a new Application. For a revision it also returns the definitions
// that revision recorded, so the base renders as what is running rather
// than as today's definitions would render it. It also returns the live
// Application, or nil, so the token can bind the exact object an apply
// must still find. The allowlist was checked by parsePlanInput.
func (d *deps) loadBase(ctx context.Context, cli client.Client, app *v1beta1.Application, revision string) (*v1beta1.Application, string, []*unstructured.Unstructured, *v1beta1.Application, error) {
	live := &v1beta1.Application{}
	err := cli.Get(ctx, client.ObjectKey{Namespace: app.Namespace, Name: app.Name}, live)
	switch {
	case apierrors.IsNotFound(err):
		live = nil
	case err != nil:
		return nil, "", nil, nil, fmt.Errorf("get application %s/%s: %w", app.Namespace, app.Name, err)
	}
	if revision == "" {
		switch {
		case live == nil:
			return nil, baseNew, nil, nil, nil
		case live.Status.LatestRevision == nil:
			// Exists but was never rendered: compare with its current spec so
			// the reader sees what changes relative to what was submitted.
			return live, "spec of " + live.Name + " (no revision yet)", nil, live, nil
		}
		revision = live.Status.LatestRevision.Name
	}
	rev := &v1beta1.ApplicationRevision{}
	if err := cli.Get(ctx, client.ObjectKey{Namespace: app.Namespace, Name: revision}, rev); err != nil {
		return nil, "", nil, nil, fmt.Errorf("get ApplicationRevision %s/%s: %w", app.Namespace, revision, err)
	}
	if !revisionOf(rev, app.Name) {
		return nil, "", nil, nil, fmt.Errorf("ApplicationRevision %s/%s belongs to another Application, not %s", app.Namespace, revision, app.Name)
	}
	defs, err := revisionDefinitions(rev)
	if err != nil {
		return nil, "", nil, nil, fmt.Errorf("read definitions recorded in ApplicationRevision %s/%s: %w", app.Namespace, revision, err)
	}
	return baseFromRevision(rev, app), rev.Name, defs, live, nil
}

// renderedSteps are the workflow step types whose effects the dry-run
// render already shows, or that write nothing.
var renderedSteps = map[string]bool{"deploy": true, "apply-component": true, "builtin-apply-component": true, "apply-application": true, "suspend": true, "step-group": true}

// opaqueSteps lists the declared step types the render cannot see into.
func opaqueSteps(app *v1beta1.Application) []string {
	if app.Spec.Workflow == nil {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	note := func(t string) {
		if !renderedSteps[t] && !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	}
	for _, s := range app.Spec.Workflow.Steps {
		note(s.Type)
		for _, sub := range s.SubSteps {
			note(sub.Type)
		}
	}
	sort.Strings(out)
	return out
}

// renderedNamespaces lists the namespaces a render writes objects into.
func renderedNamespaces(r *rendered) []string {
	seen := map[string]bool{}
	var out []string
	for _, rr := range append(append([]RenderedResource{}, r.resources...), r.policyResources...) {
		if rr.Namespace != "" && !seen[rr.Namespace] {
			seen[rr.Namespace] = true
			out = append(out, rr.Namespace)
		}
	}
	sort.Strings(out)
	return out
}

// revisionOf reports whether a revision records the named Application: by
// the label the controller sets, else by the <name>-v<N> naming it uses.
func revisionOf(rev *v1beta1.ApplicationRevision, name string) bool {
	if owner, ok := rev.Labels[oam.LabelAppName]; ok {
		return owner == name
	}
	return strings.HasPrefix(rev.Name, name+"-v")
}

// revisionDefinitions returns the definition snapshots a revision recorded,
// as objects the dry-run parser uses instead of the cluster's copies.
func revisionDefinitions(rev *v1beta1.ApplicationRevision) ([]*unstructured.Unstructured, error) {
	var objs []client.Object
	for _, def := range rev.Spec.ComponentDefinitions {
		if def != nil {
			def := def.DeepCopy()
			def.TypeMeta = metav1.TypeMeta{APIVersion: v1beta1.SchemeGroupVersion.String(), Kind: v1beta1.ComponentDefinitionKind}
			objs = append(objs, def)
		}
	}
	for _, def := range rev.Spec.TraitDefinitions {
		if def != nil {
			def := def.DeepCopy()
			def.TypeMeta = metav1.TypeMeta{APIVersion: v1beta1.SchemeGroupVersion.String(), Kind: v1beta1.TraitDefinitionKind}
			objs = append(objs, def)
		}
	}
	for _, def := range rev.Spec.PolicyDefinitions {
		def := def.DeepCopy()
		def.TypeMeta = metav1.TypeMeta{APIVersion: v1beta1.SchemeGroupVersion.String(), Kind: v1beta1.PolicyDefinitionKind}
		objs = append(objs, def)
	}
	for _, def := range rev.Spec.WorkflowStepDefinitions {
		if def != nil {
			def := def.DeepCopy()
			def.TypeMeta = metav1.TypeMeta{APIVersion: v1beta1.SchemeGroupVersion.String(), Kind: v1beta1.WorkflowStepDefinitionKind}
			objs = append(objs, def)
		}
	}
	for _, def := range rev.Spec.WorkloadDefinitions {
		def := def.DeepCopy()
		def.TypeMeta = metav1.TypeMeta{APIVersion: v1beta1.SchemeGroupVersion.String(), Kind: v1beta1.WorkloadDefinitionKind}
		objs = append(objs, def)
	}
	for _, def := range rev.Spec.SourceDefinitions {
		if def != nil {
			def := def.DeepCopy()
			def.TypeMeta = metav1.TypeMeta{APIVersion: v1beta1.SchemeGroupVersion.String(), Kind: v1beta1.SourceDefinitionKind}
			objs = append(objs, def)
		}
	}
	out := make([]*unstructured.Unstructured, 0, len(objs))
	for _, o := range objs {
		m, err := runtime.DefaultUnstructuredConverter.ToUnstructured(o)
		if err != nil {
			return nil, err
		}
		u := &unstructured.Unstructured{Object: m}
		u.SetResourceVersion("")
		out = append(out, u)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].GetKind()+"/"+out[i].GetName() < out[j].GetKind()+"/"+out[j].GetName()
	})
	return out, nil
}

// driftedDefinitions names the recorded definitions whose spec differs from
// the cluster's current copy. A definition that can no longer be read is
// left out: the render of the target reports it if it matters.
func driftedDefinitions(ctx context.Context, cli client.Client, recorded []*unstructured.Unstructured) []string {
	var drifted []string
	for _, def := range recorded {
		current := &unstructured.Unstructured{}
		current.SetGroupVersionKind(def.GroupVersionKind())
		if err := cli.Get(ctx, client.ObjectKey{Namespace: def.GetNamespace(), Name: def.GetName()}, current); err != nil {
			continue
		}
		if !equality.Semantic.DeepEqual(def.Object["spec"], current.Object["spec"]) {
			drifted = append(drifted, def.GetKind()+" "+def.GetName())
		}
	}
	return drifted
}

// baseFromRevision lifts the Application a revision recorded, giving it
// the target's name and namespace so renders key identically.
func baseFromRevision(rev *v1beta1.ApplicationRevision, target *v1beta1.Application) *v1beta1.Application {
	base := rev.Spec.Application.DeepCopy()
	base.Name = target.Name
	base.Namespace = target.Namespace
	// The server's own records are not part of what the user declared, so
	// they do not take part in the comparison.
	for k := range base.Annotations {
		if strings.HasPrefix(k, mcpAnnotationPrefix) {
			delete(base.Annotations, k)
		}
	}
	return base
}

// issueToken binds the application identity, the base revision, the live
// object's resourceVersion and the exact submitted document. The document is
// hashed as bytes, so an edit after the diff, however small, invalidates the
// token, and so does any change to the live Application.
func (d *deps) issueToken(app *v1beta1.Application, document, base string, live *v1beta1.Application, cs ChangeSet, risk Risk, localDefs int, namespaces []string) (string, time.Time) {
	specHash := sha256.Sum256([]byte(document))
	liveRV, liveUID := "", ""
	if live != nil {
		liveRV, liveUID = live.ResourceVersion, string(live.UID)
	}
	payload, _ := json.Marshal(map[string]string{
		"namespace": app.Namespace,
		"name":      app.Name,
		"base":      base,
		"live":      liveRV,
		"uid":       liveUID,
		"spec":      hex.EncodeToString(specHash[:]),
		"risk":      risk.Class,
		"issued":    time.Now().UTC().Format(time.RFC3339Nano),
	})
	sum := sha256.Sum256(payload)
	token := "sha256:" + hex.EncodeToString(sum[:])
	rec := diffRecord{
		appKey:     client.ObjectKey{Namespace: app.Namespace, Name: app.Name},
		base:       base,
		specHash:   hex.EncodeToString(specHash[:]),
		liveExists: live != nil,
		liveRV:     liveRV,
		liveUID:    liveUID,
		changeSet:  cs,
		risk:       risk,
		createdAt:  time.Now(),

		localDefinitions: localDefs,
		targetNamespaces: namespaces,
		opaqueSteps:      opaqueSteps(app),
	}
	if live != nil {
		rec.liveGeneration, rec.liveMeta = live.Generation, intentMeta(live)
	}
	d.mu.Lock()
	d.diffs.put(token, rec)
	d.mu.Unlock()
	return token, rec.createdAt.Add(diffTokenTTL).UTC()
}

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
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
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
	appKey    client.ObjectKey
	base      string
	specHash  string
	risk      string
	createdAt time.Time
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

	baseApp, baseName, err := d.loadBase(ctx, cli, app, in.Revision)
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
		base, err = d.render(ctx, baseApp, defs)
		if err != nil {
			return nil, out, fmt.Errorf("render base %s: %w", baseName, err)
		}
		out.Evidence = append(out.Evidence, fmt.Sprintf("base rendered from ApplicationRevision %s/%s with the cluster's current definitions", app.Namespace, baseName))
	} else {
		out.Evidence = append(out.Evidence, fmt.Sprintf("no Application %s/%s exists; compared against nothing", app.Namespace, app.Name))
	}

	out.ChangeSet = buildChangeSet(base, target)
	out.Risk = classify(out.ChangeSet)
	out.Token, out.ExpiresAt = d.issueToken(app, in.Application, baseName, out.Risk.Class)
	out.Meta = d.meta(ctx)
	out.Meta.Source = SourceRendered
	return nil, out, nil
}

// loadBase returns the Application spec the target is compared with: the
// named revision, else the live Application's latest revision, else nil
// for a new Application. The allowlist was checked by parsePlanInput.
func (d *deps) loadBase(ctx context.Context, cli client.Client, app *v1beta1.Application, revision string) (*v1beta1.Application, string, error) {
	rev := &v1beta1.ApplicationRevision{}
	if revision != "" {
		if err := cli.Get(ctx, client.ObjectKey{Namespace: app.Namespace, Name: revision}, rev); err != nil {
			return nil, "", fmt.Errorf("get ApplicationRevision %s/%s: %w", app.Namespace, revision, err)
		}
		return baseFromRevision(rev, app), rev.Name, nil
	}
	live := &v1beta1.Application{}
	err := cli.Get(ctx, client.ObjectKey{Namespace: app.Namespace, Name: app.Name}, live)
	switch {
	case apierrors.IsNotFound(err):
		return nil, baseNew, nil
	case err != nil:
		return nil, "", fmt.Errorf("get application %s/%s: %w", app.Namespace, app.Name, err)
	case live.Status.LatestRevision == nil:
		// Exists but was never rendered: compare with its current spec so
		// the reader sees what changes relative to what was submitted.
		return live, "spec of " + live.Name + " (no revision yet)", nil
	}
	if err := cli.Get(ctx, client.ObjectKey{Namespace: app.Namespace, Name: live.Status.LatestRevision.Name}, rev); err != nil {
		return nil, "", fmt.Errorf("get ApplicationRevision %s/%s: %w", app.Namespace, live.Status.LatestRevision.Name, err)
	}
	return baseFromRevision(rev, app), rev.Name, nil
}

// baseFromRevision lifts the Application a revision recorded, giving it
// the target's name and namespace so renders key identically.
func baseFromRevision(rev *v1beta1.ApplicationRevision, target *v1beta1.Application) *v1beta1.Application {
	base := rev.Spec.Application.DeepCopy()
	base.Name = target.Name
	base.Namespace = target.Namespace
	return base
}

// issueToken binds the application identity, base revision and the exact
// submitted document. The document is hashed as bytes, so an edit after the
// diff, however small, invalidates the token.
func (d *deps) issueToken(app *v1beta1.Application, document, base, risk string) (string, time.Time) {
	specHash := sha256.Sum256([]byte(document))
	payload, _ := json.Marshal(map[string]string{
		"namespace": app.Namespace,
		"name":      app.Name,
		"base":      base,
		"spec":      hex.EncodeToString(specHash[:]),
		"risk":      risk,
		"issued":    time.Now().UTC().Format(time.RFC3339Nano),
	})
	sum := sha256.Sum256(payload)
	token := "sha256:" + hex.EncodeToString(sum[:])
	rec := diffRecord{
		appKey:    client.ObjectKey{Namespace: app.Namespace, Name: app.Name},
		base:      base,
		specHash:  hex.EncodeToString(specHash[:]),
		risk:      risk,
		createdAt: time.Now(),
	}
	d.mu.Lock()
	d.diffs.put(token, rec)
	d.mu.Unlock()
	return token, rec.createdAt.Add(diffTokenTTL).UTC()
}

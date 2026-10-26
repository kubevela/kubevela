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
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	authenticationv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
)

// Write modes, from least to most strict.
const (
	ModeDirect = "direct"
	ModeGated  = "gated"
	ModeGitOps = "gitops"

	// agentWriteModeKey tightens the mode for a namespace (label) or an
	// Application (annotation). It can only make the mode stricter.
	agentWriteModeKey = "app.oam.dev/agent-write-mode"

	annoInitiator = mcpAnnotationPrefix + "initiator"
	annoDiffToken = mcpAnnotationPrefix + "diff-token"
	annoRisk      = mcpAnnotationPrefix + "risk"

	// maxConfirmResources and maxConfirmFields bound the confirmation text.
	maxConfirmResources = 20
	maxConfirmFields    = 3
)

var modeRank = map[string]int{ModeDirect: 0, ModeGated: 1, ModeGitOps: 2}

// WhoAmIFunc reports the caller's username. Options.WhoAmI lets tests
// supply one; the default is a SelfSubjectReview.
type WhoAmIFunc func(ctx context.Context, cli client.Client) (string, error)

// ApplyInput is what apply_application needs: the document that was
// diffed, the token the diff returned, and the mode the agent asks for.
type ApplyInput struct {
	Application string `json:"application" jsonschema:"The same Application document that was passed to diff_application, byte for byte."`
	Namespace   string `json:"namespace,omitempty" jsonschema:"Namespace when the document has none, as for diff_application."`
	Token       string `json:"token" jsonschema:"The token diff_application returned for this document."`
	Mode        string `json:"mode,omitempty" jsonschema:"Requested write mode: gitops, gated or direct. Default gated. Policy may make it stricter, never looser."`
}

// ApplyOutput is the structured result of apply_application.
type ApplyOutput struct {
	Application string `json:"application"`
	Namespace   string `json:"namespace"`
	// Mode is the mode actually used after policy.
	Mode string `json:"mode" jsonschema:"The mode actually used after policy: gitops or direct. Report it to the user."`
	// ModeReasons explains every step from the requested mode to Mode.
	ModeReasons []string `json:"modeReasons,omitempty" jsonschema:"Why the mode differs from the one requested, one line per policy that applied."`
	// Applied is true only when the cluster was written.
	Applied bool `json:"applied" jsonschema:"True only when the Application was written to the cluster."`
	// Declined is true when the user declined the confirmation prompt.
	Declined bool `json:"declined,omitempty" jsonschema:"True when the user declined the confirmation prompt; nothing was written."`
	// YAML is the document to commit in gitops mode.
	YAML string `json:"yaml,omitempty" jsonschema:"In gitops mode, the Application to commit to the delivery repository. Nothing was written to the cluster."`
	// ResourceVersion and Generation identify what was written.
	ResourceVersion string `json:"resourceVersion,omitempty"`
	Generation      int64  `json:"generation,omitempty"`
	Risk            Risk   `json:"risk" jsonschema:"Risk class and rules from the diff this token was issued for."`
	// Next is what the agent should do now.
	Next     string   `json:"next" jsonschema:"What to do next, such as open a pull request or poll get_application."`
	Evidence []string `json:"evidence"`
	Meta     Meta     `json:"meta"`
}

func (d *deps) applyApplication(ctx context.Context, req *sdk.CallToolRequest, in ApplyInput) (*sdk.CallToolResult, ApplyOutput, error) {
	out := ApplyOutput{}
	if in.Token == "" {
		return nil, out, fmt.Errorf("token is required: call diff_application on this document first and show the user its result")
	}
	if in.Mode != "" && modeRank[in.Mode] == 0 && in.Mode != ModeDirect {
		return nil, out, fmt.Errorf("unknown mode %q: use gitops, gated or direct", in.Mode)
	}
	app, _, err := d.parsePlanInput(PlanInput{Application: in.Application, Namespace: in.Namespace})
	if err != nil {
		return nil, out, err
	}
	out.Application, out.Namespace = app.Name, app.Namespace
	rec, err := d.checkToken(in.Token, app, in.Application)
	if err != nil {
		return nil, out, err
	}
	out.Risk = rec.risk
	cli := d.f.Client()
	if cli == nil {
		return nil, out, errNoConfig
	}

	live, err := d.checkUnchanged(ctx, cli, app, rec)
	if err != nil {
		return nil, out, err
	}
	out.Evidence = []string{fmt.Sprintf("diff token for %s/%s against %s", app.Namespace, app.Name, rec.base)}

	nsMode, warnings := d.namespaceMode(ctx, cli, app.Namespace)
	liveMode := ""
	if live != nil {
		liveMode = live.Annotations[agentWriteModeKey]
	}
	out.Mode, out.ModeReasons = d.decideMode(in.Mode, app.Namespace, nsMode, liveMode, app.Annotations[agentWriteModeKey], rec.risk.Class)
	out.Meta = d.meta(ctx)
	out.Meta.Warnings = append(out.Meta.Warnings, warnings...)

	if out.Mode == ModeGitOps {
		doc, err := gitopsDocument(app)
		if err != nil {
			return nil, out, err
		}
		d.burnToken(in.Token)
		out.YAML = doc
		out.Next = "Commit this YAML to the delivery repository and open a pull request with the change set and risk from diff_application. Nothing was written to the cluster."
		return nil, out, nil
	}

	// Direct: only with a confirmation the user answers in their client.
	if !clientCanElicit(req) {
		return nil, out, fmt.Errorf("direct mode needs a client that can ask the user to confirm (MCP elicitation), and this client cannot; nothing was applied. Use mode gitops instead")
	}
	answer, answered := confirmAnswer(req)
	if !answered {
		// First call: ask through the client. The protocol returns the
		// prompt as an input request and calls again with the answer; the
		// SDK does the same for clients on older protocol versions.
		nonce, err := d.promptNonce(in.Token)
		if err != nil {
			return nil, out, err
		}
		return &sdk.CallToolResult{
			InputRequests: sdk.InputRequestMap{confirmInputKey: &sdk.ElicitParams{
				Message:         confirmationText(app, rec),
				RequestedSchema: map[string]any{"type": "object", "properties": map[string]any{}},
			}},
			RequestState: nonce,
		}, out, nil
	}
	if rec.promptNonce == "" || req.Params.RequestState != rec.promptNonce {
		return nil, out, fmt.Errorf("this confirmation does not answer a prompt this server issued for the token; nothing was applied")
	}
	d.burnToken(in.Token)
	if answer.Action != "accept" {
		out.Declined = true
		out.Next = "The user declined in the confirmation prompt. Nothing was written; ask what they want to change."
		return nil, out, nil
	}

	initiator := d.initiator(ctx, cli)
	written, err := writeApplication(ctx, cli, app, live, initiator, in.Token, rec.risk.Class)
	if err != nil {
		return nil, out, err
	}
	out.Applied = true
	out.ResourceVersion, out.Generation = written.ResourceVersion, written.Generation
	out.Evidence = append(out.Evidence, fmt.Sprintf("Application %s/%s written as %s, resourceVersion %s", app.Namespace, app.Name, initiator, written.ResourceVersion))
	out.Next = "Poll get_application until the workflow finishes, then report health and endpoints."
	return nil, out, nil
}

// confirmInputKey names the confirmation in the input request map.
const confirmInputKey = "confirm"

// confirmAnswer returns the user's answer when this call carries one.
func confirmAnswer(req *sdk.CallToolRequest) (*sdk.ElicitResult, bool) {
	if req == nil || req.Params == nil || req.Params.InputResponses == nil {
		return nil, false
	}
	res, ok := req.Params.InputResponses[confirmInputKey].(*sdk.ElicitResult)
	return res, ok && res != nil
}

// promptNonce binds a fresh nonce to the token for the prompt about to be
// shown.
func (d *deps) promptNonce(token string) (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("could not prepare the confirmation prompt: %w", err)
	}
	nonce := hex.EncodeToString(buf)
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.diffs.setNonce(token, nonce) {
		return "", fmt.Errorf("token is unknown or expired: call diff_application again and show the user its result")
	}
	return nonce, nil
}

// burnToken removes a token once it has been used for an outcome.
func (d *deps) burnToken(token string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.diffs.take(token)
}

// checkToken returns the token's record after checking it belongs to this
// exact document. It does not consume the token; a mismatch leaves it
// usable for the right document.
func (d *deps) checkToken(token string, app *v1beta1.Application, document string) (diffRecord, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	rec, ok := d.diffs.lookup(token)
	if !ok {
		return diffRecord{}, fmt.Errorf("token is unknown or expired: call diff_application again and show the user its result")
	}
	if rec.appKey != (client.ObjectKey{Namespace: app.Namespace, Name: app.Name}) {
		return diffRecord{}, fmt.Errorf("token was issued for %s, not %s/%s", rec.appKey, app.Namespace, app.Name)
	}
	sum := sha256.Sum256([]byte(document))
	if hex.EncodeToString(sum[:]) != rec.specHash {
		return diffRecord{}, fmt.Errorf("token was issued for a different document: call diff_application on this one and show the user its result")
	}
	return rec, nil
}

// checkUnchanged refuses when the live Application differs from the one
// the diff saw, so a token is never applied to a state it was not computed
// for.
func (d *deps) checkUnchanged(ctx context.Context, cli client.Client, app *v1beta1.Application, rec diffRecord) (*v1beta1.Application, error) {
	live := &v1beta1.Application{}
	err := cli.Get(ctx, client.ObjectKey{Namespace: app.Namespace, Name: app.Name}, live)
	switch {
	case apierrors.IsNotFound(err):
		if rec.liveExists {
			return nil, fmt.Errorf("application %s/%s was deleted since the diff: call diff_application again", app.Namespace, app.Name)
		}
		return nil, nil
	case err != nil:
		return nil, fmt.Errorf("get application %s/%s: %w", app.Namespace, app.Name, err)
	case !rec.liveExists:
		return nil, fmt.Errorf("application %s/%s was created since the diff: call diff_application again", app.Namespace, app.Name)
	case live.Generation != rec.liveGeneration:
		return nil, fmt.Errorf("application %s/%s spec changed since the diff (generation %d, now %d): call diff_application again", app.Namespace, app.Name, rec.liveGeneration, live.Generation)
	case intentMeta(live) != rec.liveMeta:
		return nil, fmt.Errorf("application %s/%s labels or annotations changed since the diff: call diff_application again", app.Namespace, app.Name)
	}
	// Status-only writes by the controller pass. The write that follows
	// carries the resourceVersion read here, so a change landing between
	// this check and the write is still a conflict.
	return live, nil
}

// volatileAnnotations are written by controllers and tools, not by the
// person who owns the Application, so they do not count as a change of
// intent between a diff and an apply.
var volatileAnnotations = []string{
	"oam.dev/kubevela-version",
	"kubectl.kubernetes.io/last-applied-configuration",
	"app.oam.dev/last-applied-configuration",
	"app.oam.dev/last-applied-time",
}

// intentMeta is a stable digest of the labels and annotations a person
// set on an Application.
func intentMeta(app *v1beta1.Application) string {
	keep := func(m map[string]string, skip func(string) bool) []string {
		out := []string{}
		for k, v := range m {
			if !skip(k) {
				out = append(out, k+"="+v)
			}
		}
		sort.Strings(out)
		return out
	}
	labels := keep(app.Labels, func(string) bool { return false })
	annos := keep(app.Annotations, func(k string) bool {
		if strings.HasPrefix(k, mcpAnnotationPrefix) {
			return true
		}
		for _, v := range volatileAnnotations {
			if k == v {
				return true
			}
		}
		return false
	})
	sum := sha256.Sum256([]byte(strings.Join(labels, "\n") + "\x00" + strings.Join(annos, "\n")))
	return hex.EncodeToString(sum[:])
}

// namespaceMode reads the namespace's tightening label. A read failure is a
// warning: the namespace allowlist and the direct allowlist still apply.
func (d *deps) namespaceMode(ctx context.Context, cli client.Client, namespace string) (string, []string) {
	var ns corev1.Namespace
	if err := cli.Get(ctx, client.ObjectKey{Name: namespace}, &ns); err != nil {
		if apierrors.IsNotFound(err) {
			return "", nil
		}
		return "", []string{"namespace write-mode label unavailable: " + redactLine(err.Error())}
	}
	return ns.Labels[agentWriteModeKey], nil
}

// decideMode applies the policy in the KEP: start from the requested mode,
// default gated; direct only on the direct allowlist; namespace label, live
// marker and submitted marker can only tighten; HIGH risk never goes
// direct; and gated falls back to gitops until the controller hold exists.
func (d *deps) decideMode(requested, namespace, nsMode, liveMode, docMode, risk string) (string, []string) {
	mode := requested
	if mode == "" {
		mode = ModeGated
	}
	var reasons []string
	tighten := func(to, why string) {
		if _, ok := modeRank[to]; !ok || modeRank[to] <= modeRank[mode] {
			return
		}
		reasons = append(reasons, fmt.Sprintf("%s to %s: %s", mode, to, why))
		mode = to
	}
	if mode == ModeDirect && !d.directAllowed(namespace) {
		tighten(ModeGated, fmt.Sprintf("namespace %s is not on this server's direct allowlist", namespace))
	}
	tighten(nsMode, fmt.Sprintf("namespace %s is labelled %s=%s", namespace, agentWriteModeKey, nsMode))
	tighten(liveMode, fmt.Sprintf("the live Application is annotated %s=%s", agentWriteModeKey, liveMode))
	tighten(docMode, fmt.Sprintf("the submitted Application is annotated %s=%s", agentWriteModeKey, docMode))
	if risk == RiskHigh {
		tighten(ModeGated, "HIGH risk changes are never applied directly")
	}
	if mode == ModeGated {
		tighten(ModeGitOps, "gated mode needs the controller hold, which this release does not have")
	}
	return mode, reasons
}

func (d *deps) directAllowed(namespace string) bool {
	for _, ns := range d.opts.DirectNamespaces {
		if ns == namespace {
			return true
		}
	}
	return false
}

// clientCanElicit reports whether the connected client advertised
// elicitation support at initialize.
func clientCanElicit(req *sdk.CallToolRequest) bool {
	if req == nil || req.Session == nil {
		return false
	}
	params := req.Session.InitializeParams()
	return params != nil && params.Capabilities != nil && params.Capabilities.Elicitation != nil
}

// confirmationText is the prompt the user sees. The server writes it from
// the change set it hashed into the token, so a model cannot misdescribe
// the change.
func confirmationText(app *v1beta1.Application, rec diffRecord) string {
	var b strings.Builder
	fmt.Fprintf(&b, "vela: apply Application %s to %s, direct mode, no approval step\n", app.Name, app.Namespace)
	resources := rec.changeSet.Resources
	for i, rc := range resources {
		if i == maxConfirmResources {
			fmt.Fprintf(&b, "  ... and %d more resources\n", len(resources)-maxConfirmResources)
			break
		}
		id := rc.Kind + " " + rc.Name
		if rc.Namespace != "" {
			id = rc.Kind + " " + rc.Namespace + "/" + rc.Name
		}
		fmt.Fprintf(&b, "  %s %s\n", rc.Action, id)
		for j, f := range rc.Fields {
			if j == maxConfirmFields {
				fmt.Fprintf(&b, "    ... and %d more fields\n", len(rc.Fields)-maxConfirmFields)
				break
			}
			fmt.Fprintf(&b, "    %s  %v -> %v\n", f.Path, valueOrNone(f.From), valueOrNone(f.To))
		}
	}
	if len(resources) == 0 {
		b.WriteString("  no rendered resources change\n")
	}
	rules := make([]string, 0, len(rec.risk.Rules))
	for _, r := range rec.risk.Rules {
		rules = append(rules, r.Rule)
	}
	fmt.Fprintf(&b, "  Risk %s: %s\n", rec.risk.Class, strings.Join(uniq(rules), ", "))
	fmt.Fprintf(&b, "  Base %s. Token valid for %s more.", rec.base, time.Until(rec.createdAt.Add(diffTokenTTL)).Round(time.Minute))
	return redactLine(b.String())
}

func valueOrNone(v any) any {
	if v == nil {
		return "(none)"
	}
	return v
}

func uniq(in []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// gitopsDocument renders the Application to commit: the submitted spec and
// metadata, with no status and no server-owned annotations.
func gitopsDocument(app *v1beta1.Application) (string, error) {
	doc := &v1beta1.Application{}
	doc.APIVersion = v1beta1.SchemeGroupVersion.String()
	doc.Kind = v1beta1.ApplicationKind
	doc.Name, doc.Namespace = app.Name, app.Namespace
	doc.Labels = app.Labels
	doc.Annotations = app.Annotations
	doc.Spec = app.Spec
	raw, err := yaml.Marshal(doc)
	if err != nil {
		return "", fmt.Errorf("render gitops document: %w", err)
	}
	lines := []string{}
	for _, l := range strings.Split(string(raw), "\n") {
		if strings.TrimSpace(l) == "creationTimestamp: null" || strings.HasPrefix(l, "status:") {
			continue
		}
		lines = append(lines, l)
	}
	return strings.TrimRight(strings.Join(lines, "\n"), "\n") + "\n", nil
}

// writeApplication creates a new Application or updates the live one with
// the submitted spec. The update carries the live resourceVersion the diff
// saw, so a change that lands between the check and the write is a
// conflict, never an overwrite. Annotations the live object has and the
// document does not are kept; server-owned ones are replaced.
func writeApplication(ctx context.Context, cli client.Client, app, live *v1beta1.Application, initiator, token, risk string) (*v1beta1.Application, error) {
	stamp := func(annos map[string]string) map[string]string {
		if annos == nil {
			annos = map[string]string{}
		}
		for k := range annos {
			if strings.HasPrefix(k, mcpAnnotationPrefix) {
				delete(annos, k)
			}
		}
		annos[annoInitiator] = initiator
		annos[annoDiffToken] = token
		annos[annoRisk] = risk
		annos[oam.AnnotationFilterAnnotationKeys] = withFilteredKeys(annos[oam.AnnotationFilterAnnotationKeys], annoInitiator, annoDiffToken, annoRisk)
		return annos
	}
	if live == nil {
		obj := app.DeepCopy()
		obj.ResourceVersion = ""
		obj.Status = v1beta1.Application{}.Status
		obj.Annotations = stamp(obj.Annotations)
		if err := cli.Create(ctx, obj); err != nil {
			return nil, fmt.Errorf("create application %s/%s: %w", app.Namespace, app.Name, err)
		}
		return obj, nil
	}
	obj := live.DeepCopy()
	obj.Spec = app.Spec
	if obj.Labels == nil {
		obj.Labels = map[string]string{}
	}
	for k, v := range app.Labels {
		obj.Labels[k] = v
	}
	merged := map[string]string{}
	for k, v := range live.Annotations {
		merged[k] = v
	}
	for k, v := range app.Annotations {
		merged[k] = v
	}
	obj.Annotations = stamp(merged)
	if err := cli.Update(ctx, obj); err != nil {
		if apierrors.IsConflict(err) {
			return nil, fmt.Errorf("application %s/%s changed while applying; nothing was written: call diff_application again", app.Namespace, app.Name)
		}
		return nil, fmt.Errorf("update application %s/%s: %w", app.Namespace, app.Name, err)
	}
	return obj, nil
}

// withFilteredKeys adds keys to a filter.oam.dev/annotation-keys value,
// keeping the user's entries and their order. KubeVela copies an
// Application's annotations onto every resource it renders unless they are
// listed there, and the server's records belong on the Application only.
func withFilteredKeys(current string, keys ...string) string {
	out := []string{}
	seen := map[string]bool{}
	for _, k := range append(strings.Split(current, ","), keys...) {
		k = strings.TrimSpace(k)
		if k == "" || seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, k)
	}
	return strings.Join(out, ",")
}

// initiator names who applied the change, for the record on the object.
func (d *deps) initiator(ctx context.Context, cli client.Client) string {
	name, err := d.opts.WhoAmI(ctx, cli)
	if err != nil || name == "" {
		return "unknown"
	}
	return name
}

// selfSubjectReview is the default WhoAmIFunc, the same check as
// kubectl auth whoami.
func selfSubjectReview(ctx context.Context, cli client.Client) (string, error) {
	review := &authenticationv1.SelfSubjectReview{}
	if err := cli.Create(ctx, review); err != nil {
		return "", err
	}
	if review.Status.UserInfo.Username == "" {
		return "", errors.New("empty username")
	}
	return review.Status.UserInfo.Username, nil
}

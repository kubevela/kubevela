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
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	monitorContext "github.com/kubevela/pkg/monitor/context"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

// secretLeakTestClient builds a fake client and an AppHandler for exercising the
// policy observability persistence path in isolation (no envtest / API server).
func secretLeakTestClient(t *testing.T, objs ...runtime.Object) client.Client {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatalf("add clientgo scheme: %v", err)
	}
	return fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objs...).Build()
}

func secretLeakTestApp() *v1beta1.Application {
	return &v1beta1.Application{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "leak-app",
			Namespace: "leak-ns",
			UID:       types.UID("app-uid-1234"),
		},
	}
}

// The literal secret value that a policy reads out of a Kubernetes Secret (e.g. via
// kube.#Read) and surfaces into policy context. It must never appear in a ConfigMap.
const leakedSecretValue = "s3cr3t-db-password-DO-NOT-LEAK"

// TestPolicyContext_SecretLeaksIntoConfigMap documents the vulnerability from
// kubevela/kubevela#6840: before the fix there was NO way to keep policy context
// out of the plaintext observability ConfigMap, so a secret value surfaced through
// output.ctx is persisted verbatim where any user with ConfigMap read access can
// see it. This test asserts the leak on the plain-ctx (non-sensitive) channel and
// therefore stays green after the fix — it is the reference "before" behavior.
func TestPolicyContext_SecretLeaksIntoConfigMap(t *testing.T) {
	cli := secretLeakTestClient(t)
	h := &AppHandler{Client: cli}
	app := secretLeakTestApp()

	// A policy that read a Secret and (incorrectly) placed the value in output.ctx.
	results := []RenderedPolicyResult{
		{
			PolicyName: "read-db-secret",
			Enabled:    true,
			Transforms: &PolicyOutput{
				Ctx: map[string]interface{}{"dbPassword": leakedSecretValue},
			},
			AdditionalContext: map[string]interface{}{"dbPassword": leakedSecretValue},
		},
	}

	monCtx := monitorContext.NewTraceContext(context.Background(), "leak-repro")
	h.writePolicyObservabilityConfigMap(monCtx, app, results, &v1beta1.ApplicationSpec{}, nil, false, false)

	cm := &corev1.ConfigMap{}
	if err := cli.Get(context.Background(), client.ObjectKey{
		Name:      policyConfigMapName(app.Namespace, app.Name),
		Namespace: app.Namespace,
	}, cm); err != nil {
		t.Fatalf("expected observability ConfigMap to exist: %v", err)
	}

	if !configMapContains(cm, leakedSecretValue) {
		t.Fatalf("expected the plain output.ctx value to be present in the ConfigMap (documents the leak channel)")
	}
	t.Logf("LEAK CONFIRMED: secret value present in ConfigMap %q via output.ctx", cm.Name)
}

// TestPolicyContext_SensitiveCtxNotInConfigMap verifies the fix: a policy that
// routes secret-derived values through output.sensitiveCtx keeps them OUT of the
// ConfigMap and stores them in an Application-owned Secret instead.
func TestPolicyContext_SensitiveCtxNotInConfigMap(t *testing.T) {
	cli := secretLeakTestClient(t)
	h := &AppHandler{Client: cli}
	app := secretLeakTestApp()

	results := []RenderedPolicyResult{
		{
			PolicyName: "read-db-secret",
			Enabled:    true,
			Transforms: &PolicyOutput{
				// Non-sensitive context stays in ctx (still allowed in the ConfigMap).
				Ctx:          map[string]interface{}{"dbHost": "db.internal"},
				SensitiveCtx: map[string]interface{}{"dbPassword": leakedSecretValue},
			},
			AdditionalContext: map[string]interface{}{"dbHost": "db.internal"},
			SensitiveContext:  map[string]interface{}{"dbPassword": leakedSecretValue},
		},
	}

	monCtx := monitorContext.NewTraceContext(context.Background(), "leak-fixed")
	h.writePolicyObservabilityConfigMap(monCtx, app, results, &v1beta1.ApplicationSpec{}, nil, false, false)

	// 1. The ConfigMap must NOT contain the sensitive value...
	cm := &corev1.ConfigMap{}
	if err := cli.Get(context.Background(), client.ObjectKey{
		Name:      policyConfigMapName(app.Namespace, app.Name),
		Namespace: app.Namespace,
	}, cm); err != nil {
		t.Fatalf("expected observability ConfigMap to exist: %v", err)
	}
	if configMapContains(cm, leakedSecretValue) {
		t.Fatalf("SECURITY REGRESSION: sensitive value leaked into ConfigMap %q", cm.Name)
	}
	// ...but non-sensitive context is still persisted for observability.
	if !configMapContains(cm, "db.internal") {
		t.Fatalf("expected non-sensitive ctx value to remain in the ConfigMap")
	}

	// 2. The sensitive value must be stored in an Application-owned Secret.
	secret := &corev1.Secret{}
	if err := cli.Get(context.Background(), client.ObjectKey{
		Name:      policySecretName(app.Namespace, app.Name),
		Namespace: app.Namespace,
	}, secret); err != nil {
		t.Fatalf("expected sensitive-context Secret to exist: %v", err)
	}
	if !secretContains(secret, leakedSecretValue) {
		t.Fatalf("expected sensitive value to be stored in the Secret")
	}

	// 3. The Secret must be owned by the Application (garbage-collected on delete).
	if !ownedByApplication(secret.OwnerReferences, app) {
		t.Fatalf("expected Secret to carry an owner reference to the Application for GC")
	}
	if secret.Type != corev1.SecretTypeOpaque {
		t.Fatalf("expected Opaque Secret, got %q", secret.Type)
	}
}

// TestPolicyContext_SensitiveCtxClearedWhenEmpty verifies that once sensitive
// context stops being produced (sensitiveCtx removed / policy disabled), the
// previously stored credentials are cleared from the owned Secret rather than
// lingering indefinitely.
func TestPolicyContext_SensitiveCtxClearedWhenEmpty(t *testing.T) {
	cli := secretLeakTestClient(t)
	h := &AppHandler{Client: cli}
	app := secretLeakTestApp()

	// First reconcile: policy contributes a secret → Secret is populated.
	withSensitive := []RenderedPolicyResult{{
		PolicyName:       "read-db-secret",
		Enabled:          true,
		Transforms:       &PolicyOutput{SensitiveCtx: map[string]interface{}{"dbPassword": leakedSecretValue}},
		SensitiveContext: map[string]interface{}{"dbPassword": leakedSecretValue},
	}}
	monCtx := monitorContext.NewTraceContext(context.Background(), "clear-1")
	h.writePolicyObservabilityConfigMap(monCtx, app, withSensitive, &v1beta1.ApplicationSpec{}, nil, false, false)

	secret := &corev1.Secret{}
	key := client.ObjectKey{Name: policySecretName(app.Namespace, app.Name), Namespace: app.Namespace}
	if err := cli.Get(context.Background(), key, secret); err != nil {
		t.Fatalf("expected Secret to exist after first reconcile: %v", err)
	}
	if !secretContains(secret, leakedSecretValue) {
		t.Fatalf("expected sensitive value stored after first reconcile")
	}

	// Second reconcile: policy no longer contributes sensitive context.
	noSensitive := []RenderedPolicyResult{{
		PolicyName: "read-db-secret",
		Enabled:    true,
		Transforms: &PolicyOutput{Ctx: map[string]interface{}{"dbHost": "db.internal"}},
	}}
	monCtx = monitorContext.NewTraceContext(context.Background(), "clear-2")
	h.writePolicyObservabilityConfigMap(monCtx, app, noSensitive, &v1beta1.ApplicationSpec{}, nil, false, false)

	if err := cli.Get(context.Background(), key, secret); err != nil {
		t.Fatalf("expected Secret to still exist (cleared, not deleted): %v", err)
	}
	if len(secret.Data) != 0 {
		t.Fatalf("expected Secret Data to be cleared, still had %d entries", len(secret.Data))
	}
}

// TestPolicyContext_DoesNotOverwriteForeignSecret verifies the controller refuses
// to clobber a Secret it does not own even on a name collision.
func TestPolicyContext_DoesNotOverwriteForeignSecret(t *testing.T) {
	app := secretLeakTestApp()
	foreign := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      policySecretName(app.Namespace, app.Name),
			Namespace: app.Namespace,
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{"unrelated": []byte("keep-me")},
	}
	cli := secretLeakTestClient(t, foreign)

	err := reconcilePolicySecret(context.Background(), cli, app, map[string][]byte{"001-p": []byte("new")})
	if err == nil {
		t.Fatalf("expected reconcile to refuse overwriting a non-owned Secret")
	}

	got := &corev1.Secret{}
	if getErr := cli.Get(context.Background(), client.ObjectKey{Name: foreign.Name, Namespace: foreign.Namespace}, got); getErr != nil {
		t.Fatalf("foreign Secret should still exist: %v", getErr)
	}
	if string(got.Data["unrelated"]) != "keep-me" {
		t.Fatalf("foreign Secret data must be untouched")
	}
}

// TestPolicyContext_NoSecretWhenNoSensitiveData ensures we don't create empty
// Secrets when no policy contributes sensitive context (no needless RBAC use).
func TestPolicyContext_NoSecretWhenNoSensitiveData(t *testing.T) {
	cli := secretLeakTestClient(t)
	h := &AppHandler{Client: cli}
	app := secretLeakTestApp()

	results := []RenderedPolicyResult{
		{
			PolicyName:        "plain-policy",
			Enabled:           true,
			Transforms:        &PolicyOutput{Ctx: map[string]interface{}{"region": "us-east-1"}},
			AdditionalContext: map[string]interface{}{"region": "us-east-1"},
		},
	}

	monCtx := monitorContext.NewTraceContext(context.Background(), "no-sensitive")
	h.writePolicyObservabilityConfigMap(monCtx, app, results, &v1beta1.ApplicationSpec{}, nil, false, false)

	secret := &corev1.Secret{}
	err := cli.Get(context.Background(), client.ObjectKey{
		Name:      policySecretName(app.Namespace, app.Name),
		Namespace: app.Namespace,
	}, secret)
	if err == nil {
		t.Fatalf("did not expect a sensitive-context Secret when no sensitive data is present")
	}
}

// TestExtractOutput_AllowsSensitiveCtx verifies the CUE contract accepts the new
// output.sensitiveCtx marker field.
func TestExtractOutput_AllowsSensitiveCtx(t *testing.T) {
	h := &AppHandler{}
	val := compileCUEForTest(t, `output: { ctx: { a: "1" }, sensitiveCtx: { token: "abc" } }`)
	out, err := h.extractOutput(val)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out == nil {
		t.Fatalf("expected non-nil output")
	}
	if got := out.SensitiveCtx["token"]; got != "abc" {
		t.Fatalf("expected sensitiveCtx.token=abc, got %v", got)
	}
}

func compileCUEForTest(t *testing.T, src string) cue.Value {
	t.Helper()
	val := cuecontext.New().CompileString(src)
	if err := val.Err(); err != nil {
		t.Fatalf("failed to compile CUE: %v", err)
	}
	return val
}

func configMapContains(cm *corev1.ConfigMap, needle string) bool {
	for _, v := range cm.Data {
		if strings.Contains(v, needle) {
			return true
		}
	}
	return false
}

func secretContains(secret *corev1.Secret, needle string) bool {
	for _, v := range secret.Data {
		if strings.Contains(string(v), needle) {
			return true
		}
	}
	return false
}

func ownedByApplication(refs []metav1.OwnerReference, app *v1beta1.Application) bool {
	for _, ref := range refs {
		if ref.Kind == v1beta1.ApplicationKind && ref.Name == app.Name && ref.UID == app.UID {
			return true
		}
	}
	return false
}

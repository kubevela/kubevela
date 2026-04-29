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
	"encoding/json"
	"errors"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/kubevela/pkg/util/singleton"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

func TestComputeValuesFromContentFingerprint_NilApp(t *testing.T) {
	got, err := computeValuesFromContentFingerprint(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "" {
		t.Fatalf("expected empty fingerprint for nil app, got %q", got)
	}
}

func TestComputeValuesFromContentFingerprint_EmptyComponents(t *testing.T) {
	app := &v1beta1.Application{}
	got, err := computeValuesFromContentFingerprint(context.Background(), app)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "" {
		t.Fatalf("expected empty fingerprint for app with no components, got %q", got)
	}
}

func TestComputeValuesFromContentFingerprint_NoHelmchartComponents(t *testing.T) {
	app := &v1beta1.Application{
		Spec: v1beta1.ApplicationSpec{
			Components: []common.ApplicationComponent{
				{Name: "web", Type: "webservice"},
			},
		},
	}
	got, err := computeValuesFromContentFingerprint(context.Background(), app)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "" {
		t.Fatalf("expected empty fingerprint, got %q", got)
	}
}

func TestComputeValuesFromContentFingerprint_HelmchartNoValuesFrom(t *testing.T) {
	app := &v1beta1.Application{
		Spec: v1beta1.ApplicationSpec{
			Components: []common.ApplicationComponent{
				{
					Name: "x",
					Type: "helmchart",
					Properties: &runtime.RawExtension{
						Raw: []byte(`{"chart":{"source":"foo"}}`),
					},
				},
			},
		},
	}
	got, err := computeValuesFromContentFingerprint(context.Background(), app)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "" {
		t.Fatalf("expected empty fingerprint when valuesFrom is absent, got %q", got)
	}
}

func TestComputeValuesFromContentFingerprint_HelmchartNilProperties(t *testing.T) {
	app := &v1beta1.Application{
		Spec: v1beta1.ApplicationSpec{
			Components: []common.ApplicationComponent{
				{Name: "x", Type: "helmchart", Properties: nil},
			},
		},
	}
	got, err := computeValuesFromContentFingerprint(context.Background(), app)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "" {
		t.Fatalf("expected empty fingerprint, got %q", got)
	}
}

// newAppWithCMValuesFrom builds an Application with one helmchart component
// referencing a single ConfigMap valuesFrom source.
func newAppWithCMValuesFrom(t *testing.T, appName, ns, cmName, key string) *v1beta1.Application {
	t.Helper()
	entry := map[string]interface{}{"kind": "ConfigMap", "name": cmName}
	if key != "" {
		entry["key"] = key
	}
	props := map[string]interface{}{
		"chart":      map[string]interface{}{"source": "foo"},
		"release":    map[string]interface{}{"namespace": ns},
		"valuesFrom": []map[string]interface{}{entry},
	}
	raw, err := json.Marshal(props)
	if err != nil {
		t.Fatalf("marshal props: %v", err)
	}
	return &v1beta1.Application{
		ObjectMeta: metav1.ObjectMeta{Name: appName, Namespace: ns},
		Spec: v1beta1.ApplicationSpec{
			Components: []common.ApplicationComponent{
				{Name: "x", Type: "helmchart", Properties: &runtime.RawExtension{Raw: raw}},
			},
		},
	}
}

// setupFakeClient installs a fresh fake controller-runtime client into the
// kubevela singleton.KubeClient slot, seeded with the given runtime objects.
// Tests that need a different object set call this again to swap the client.
//
// Cleanup deliberately re-Sets the singleton to nil rather than capturing a
// "previous" value with Get() because the singleton's loader chain calls
// config.GetConfigOrDie(), which os.Exits when run outside an envtest context
// (i.e. `go test -run TestComputeValuesFrom...` directly, without the package's
// Ginkgo suite). Calling Get() in setupFakeClient would therefore hard-exit
// the test binary in standalone runs.
//
// Why this is safe in the package binary: Go runs Test* functions in
// alphabetical order, so `TestAPIs` (the Ginkgo entry point) runs FIRST and
// has fully completed BeforeSuite/AfterSuite by the time the standalone
// `TestComputeValuesFromContentFingerprint_*` functions begin. None of the
// Ginkgo specs in this package use setupFakeClient, and after the standalone
// Test* functions finish, no further code in the binary calls
// singleton.KubeClient.Get(). The cleanup leaves the singleton at nil at exit,
// which is harmless. If a future Ginkgo spec ever needs a fake client at
// suite-time, it must restore the envtest client itself rather than rely on
// this helper.
func setupFakeClient(t *testing.T, objs ...runtime.Object) {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatalf("add scheme: %v", err)
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objs...).Build()
	t.Cleanup(func() { singleton.KubeClient.Set(nil) })
	singleton.KubeClient.Set(c)
}

func TestComputeValuesFromContentFingerprint_SingleConfigMap(t *testing.T) {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "cfg", Namespace: "ns1"},
		Data:       map[string]string{"values.yaml": "replicas: 3\n"},
	}
	setupFakeClient(t, cm)

	app := newAppWithCMValuesFrom(t, "app1", "ns1", "cfg", "")
	got, err := computeValuesFromContentFingerprint(context.Background(), app)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == "" {
		t.Fatalf("expected non-empty fingerprint, got empty")
	}
	if len(got) != 64 {
		t.Fatalf("expected 64-char sha256 hex, got %d chars: %q", len(got), got)
	}
}

func TestComputeValuesFromContentFingerprint_MalformedProperties(t *testing.T) {
	// Backstop the silent-continue at the json.Unmarshal step — malformed
	// helmchart properties must NOT propagate as an error. The render layer
	// is the source of truth for schema validation; gating the workflow on
	// a parser disagreement here would block reconciles unnecessarily.
	app := &v1beta1.Application{
		Spec: v1beta1.ApplicationSpec{
			Components: []common.ApplicationComponent{
				{
					Name:       "x",
					Type:       "helmchart",
					Properties: &runtime.RawExtension{Raw: []byte(`{not valid json`)},
				},
			},
		},
	}
	got, err := computeValuesFromContentFingerprint(context.Background(), app)
	if err != nil {
		t.Fatalf("malformed properties must not return error, got: %v", err)
	}
	if got != "" {
		t.Fatalf("expected empty fingerprint for malformed properties, got %q", got)
	}
}

func TestComputeValuesFromContentFingerprint_Deterministic(t *testing.T) {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "cfg", Namespace: "ns1"},
		Data:       map[string]string{"values.yaml": "replicas: 3\nimage:\n  tag: v1\n"},
	}
	setupFakeClient(t, cm)
	app := newAppWithCMValuesFrom(t, "app1", "ns1", "cfg", "")

	first, err := computeValuesFromContentFingerprint(context.Background(), app)
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	for i := 0; i < 5; i++ {
		got, err := computeValuesFromContentFingerprint(context.Background(), app)
		if err != nil {
			t.Fatalf("iteration %d: %v", i, err)
		}
		if got != first {
			t.Fatalf("iteration %d: fingerprint moved: %q vs %q", i, got, first)
		}
	}
}

func TestComputeValuesFromContentFingerprint_ContentChangeMovesFingerprint(t *testing.T) {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "cfg", Namespace: "ns1"},
		Data:       map[string]string{"values.yaml": "replicas: 3\n"},
	}
	setupFakeClient(t, cm)
	app := newAppWithCMValuesFrom(t, "app1", "ns1", "cfg", "")

	before, err := computeValuesFromContentFingerprint(context.Background(), app)
	if err != nil {
		t.Fatalf("before: %v", err)
	}

	// Edit the CM content and re-seed the fake client. setupFakeClient builds
	// a fresh client from the supplied objects, so this is a faithful
	// "operator edited the CM and the next reconcile re-reads it" simulation.
	cm.Data["values.yaml"] = "replicas: 5\n"
	setupFakeClient(t, cm)

	after, err := computeValuesFromContentFingerprint(context.Background(), app)
	if err != nil {
		t.Fatalf("after: %v", err)
	}

	if before == after {
		t.Fatalf("expected fingerprint to move on content change, both = %q", before)
	}
}

func TestComputeValuesFromContentFingerprint_WhitespaceOnlyChangeStable(t *testing.T) {
	cmA := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "cfg", Namespace: "ns1"},
		Data: map[string]string{
			"values.yaml": "replicas: 3\nimage:\n  tag: v1\n",
		},
	}
	cmB := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "cfg", Namespace: "ns1"},
		Data: map[string]string{
			// Same parsed shape, different formatting — leading comment and
			// extra blank line. canonicalLine parses YAML and re-marshals as
			// JSON, so cosmetic edits like these must NOT move the digest.
			"values.yaml": "# leading comment\nreplicas: 3\n\nimage:\n  tag: v1\n",
		},
	}

	setupFakeClient(t, cmA)
	app := newAppWithCMValuesFrom(t, "app1", "ns1", "cfg", "")
	first, err := computeValuesFromContentFingerprint(context.Background(), app)
	if err != nil {
		t.Fatalf("first: %v", err)
	}

	setupFakeClient(t, cmB)
	second, err := computeValuesFromContentFingerprint(context.Background(), app)
	if err != nil {
		t.Fatalf("second: %v", err)
	}

	if first != second {
		t.Fatalf("expected whitespace-only YAML change to leave fingerprint stable, got %q vs %q", first, second)
	}
}

// newAppWithSecretValuesFrom builds an Application with one helmchart
// component referencing a single Secret valuesFrom source.
func newAppWithSecretValuesFrom(t *testing.T, appName, ns, secretName, key string) *v1beta1.Application {
	t.Helper()
	entry := map[string]interface{}{"kind": "Secret", "name": secretName}
	if key != "" {
		entry["key"] = key
	}
	props := map[string]interface{}{
		"chart":      map[string]interface{}{"source": "foo"},
		"release":    map[string]interface{}{"namespace": ns},
		"valuesFrom": []map[string]interface{}{entry},
	}
	raw, err := json.Marshal(props)
	if err != nil {
		t.Fatalf("marshal props: %v", err)
	}
	return &v1beta1.Application{
		ObjectMeta: metav1.ObjectMeta{Name: appName, Namespace: ns},
		Spec: v1beta1.ApplicationSpec{
			Components: []common.ApplicationComponent{
				{Name: "x", Type: "helmchart", Properties: &runtime.RawExtension{Raw: raw}},
			},
		},
	}
}

func TestComputeValuesFromContentFingerprint_SecretSource(t *testing.T) {
	sec := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "sec", Namespace: "ns1"},
		Data:       map[string][]byte{"values.yaml": []byte("replicas: 2\n")},
	}
	setupFakeClient(t, sec)
	app := newAppWithSecretValuesFrom(t, "app1", "ns1", "sec", "")

	got, err := computeValuesFromContentFingerprint(context.Background(), app)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == "" {
		t.Fatalf("expected non-empty fingerprint for Secret source")
	}
	if len(got) != 64 {
		t.Fatalf("expected 64-char hex, got %d", len(got))
	}
}

func TestComputeValuesFromContentFingerprint_SecretAndConfigMapDistinct(t *testing.T) {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "shared", Namespace: "ns1"},
		Data:       map[string]string{"values.yaml": "replicas: 2\n"},
	}
	sec := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "shared", Namespace: "ns1"},
		Data:       map[string][]byte{"values.yaml": []byte("replicas: 2\n")},
	}
	setupFakeClient(t, cm, sec)

	cmApp := newAppWithCMValuesFrom(t, "a", "ns1", "shared", "")
	secApp := newAppWithSecretValuesFrom(t, "a", "ns1", "shared", "")

	cmFp, err := computeValuesFromContentFingerprint(context.Background(), cmApp)
	if err != nil {
		t.Fatalf("CM fingerprint: %v", err)
	}
	secFp, err := computeValuesFromContentFingerprint(context.Background(), secApp)
	if err != nil {
		t.Fatalf("Secret fingerprint: %v", err)
	}
	if cmFp == secFp {
		t.Fatalf("ConfigMap and Secret with identical content + name should produce DIFFERENT fingerprints (kind is part of the per-source line) — got %q for both", cmFp)
	}
}

// newAppWithOptionalCMValuesFrom builds an Application with one helmchart
// component referencing a single ConfigMap valuesFrom source marked optional.
func newAppWithOptionalCMValuesFrom(t *testing.T, appName, ns, cmName string) *v1beta1.Application {
	t.Helper()
	raw, err := json.Marshal(map[string]interface{}{
		"chart":   map[string]interface{}{"source": "foo"},
		"release": map[string]interface{}{"namespace": ns},
		"valuesFrom": []map[string]interface{}{
			{"kind": "ConfigMap", "name": cmName, "optional": true},
		},
	})
	if err != nil {
		t.Fatalf("marshal props: %v", err)
	}
	return &v1beta1.Application{
		ObjectMeta: metav1.ObjectMeta{Name: appName, Namespace: ns},
		Spec: v1beta1.ApplicationSpec{
			Components: []common.ApplicationComponent{
				{Name: "x", Type: "helmchart", Properties: &runtime.RawExtension{Raw: raw}},
			},
		},
	}
}

func TestComputeValuesFromContentFingerprint_OptionalMissing_Stable(t *testing.T) {
	setupFakeClient(t) // no CM at all
	app := newAppWithOptionalCMValuesFrom(t, "app1", "ns1", "ghost")

	first, err := computeValuesFromContentFingerprint(context.Background(), app)
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	second, err := computeValuesFromContentFingerprint(context.Background(), app)
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if first == "" {
		t.Fatalf("expected a non-empty fingerprint covering the <missing> sentinel, got empty")
	}
	if first != second {
		t.Fatalf("fingerprint must be stable while optional source stays missing, got %q vs %q", first, second)
	}
}

func TestComputeValuesFromContentFingerprint_OptionalAppearMovesFingerprint(t *testing.T) {
	setupFakeClient(t) // missing
	app := newAppWithOptionalCMValuesFrom(t, "app1", "ns1", "ghost")
	missingFp, err := computeValuesFromContentFingerprint(context.Background(), app)
	if err != nil {
		t.Fatalf("missing: %v", err)
	}

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "ghost", Namespace: "ns1"},
		Data:       map[string]string{"values.yaml": "replicas: 4\n"},
	}
	setupFakeClient(t, cm)
	presentFp, err := computeValuesFromContentFingerprint(context.Background(), app)
	if err != nil {
		t.Fatalf("present: %v", err)
	}
	if missingFp == presentFp {
		t.Fatalf("expected fingerprint to move when optional source appears")
	}
}

func TestComputeValuesFromContentFingerprint_RequiredMissingErrors(t *testing.T) {
	setupFakeClient(t) // no CM
	app := newAppWithCMValuesFrom(t, "app1", "ns1", "ghost", "")
	_, err := computeValuesFromContentFingerprint(context.Background(), app)
	if err == nil {
		t.Fatalf("expected error for required missing ConfigMap, got nil")
	}
	if !strings.Contains(err.Error(), "configmap ns1/ghost not found") {
		t.Fatalf("error wording missing source identity: %v", err)
	}
}

func TestComputeValuesFromContentFingerprint_InvalidYAMLErrors(t *testing.T) {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "cfg", Namespace: "ns1"},
		Data: map[string]string{
			// Unterminated flow mapping — invalid YAML.
			"values.yaml": "replicas: [3, 4",
		},
	}
	setupFakeClient(t, cm)
	app := newAppWithCMValuesFrom(t, "app1", "ns1", "cfg", "")
	_, err := computeValuesFromContentFingerprint(context.Background(), app)
	if err == nil {
		t.Fatalf("expected YAML parse error, got nil")
	}
	if !strings.Contains(err.Error(), "invalid YAML") {
		t.Fatalf("error wording missing 'invalid YAML': %v", err)
	}
}

func TestComputeValuesFromContentFingerprint_InvalidYAMLFailsEvenIfOptional(t *testing.T) {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "cfg", Namespace: "ns1"},
		Data:       map[string]string{"values.yaml": "key: [unterminated"},
	}
	setupFakeClient(t, cm)
	app := newAppWithOptionalCMValuesFrom(t, "app1", "ns1", "cfg")
	_, err := computeValuesFromContentFingerprint(context.Background(), app)
	if err == nil {
		t.Fatalf("expected parse error to surface even with optional=true (matches helm provider semantics)")
	}
}

func TestComputeValuesFromContentFingerprint_CrossNamespaceRejected(t *testing.T) {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "leaked", Namespace: "ns-a"},
		Data:       map[string]string{"values.yaml": "x: 1\n"},
	}
	setupFakeClient(t, cm)

	props, _ := json.Marshal(map[string]interface{}{
		"chart":   map[string]interface{}{"source": "foo"},
		"release": map[string]interface{}{"namespace": "ns-b"},
		"valuesFrom": []map[string]interface{}{
			{"kind": "ConfigMap", "name": "leaked", "namespace": "ns-a"},
		},
	})
	app := &v1beta1.Application{
		ObjectMeta: metav1.ObjectMeta{Name: "tenant-b", Namespace: "ns-b"},
		Spec: v1beta1.ApplicationSpec{
			Components: []common.ApplicationComponent{
				{Name: "x", Type: "helmchart", Properties: &runtime.RawExtension{Raw: props}},
			},
		},
	}

	_, err := computeValuesFromContentFingerprint(context.Background(), app)
	if err == nil {
		t.Fatalf("expected cross-namespace rejection, got nil")
	}
	if !strings.Contains(err.Error(), "cross-namespace valuesFrom") {
		t.Fatalf("error wording missing 'cross-namespace valuesFrom': %v", err)
	}
}

func TestComputeValuesFromContentFingerprint_ExplicitSameNamespaceAllowed(t *testing.T) {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "cfg", Namespace: "ns1"},
		Data:       map[string]string{"values.yaml": "x: 1\n"},
	}
	setupFakeClient(t, cm)

	props, _ := json.Marshal(map[string]interface{}{
		"chart":   map[string]interface{}{"source": "foo"},
		"release": map[string]interface{}{"namespace": "ns1"},
		"valuesFrom": []map[string]interface{}{
			{"kind": "ConfigMap", "name": "cfg", "namespace": "ns1"},
		},
	})
	app := &v1beta1.Application{
		ObjectMeta: metav1.ObjectMeta{Name: "app1", Namespace: "ns1"},
		Spec: v1beta1.ApplicationSpec{
			Components: []common.ApplicationComponent{
				{Name: "x", Type: "helmchart", Properties: &runtime.RawExtension{Raw: props}},
			},
		},
	}

	got, err := computeValuesFromContentFingerprint(context.Background(), app)
	if err != nil {
		t.Fatalf("expected explicit same-namespace to be accepted, got error: %v", err)
	}
	if got == "" {
		t.Fatalf("expected non-empty fingerprint")
	}
}

func TestComputeValuesFromContentFingerprint_MultipleSourcesSorted(t *testing.T) {
	a := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "ns1"},
		Data:       map[string]string{"values.yaml": "x: 1\n"},
	}
	b := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "b", Namespace: "ns1"},
		Data:       map[string]string{"values.yaml": "y: 2\n"},
	}
	setupFakeClient(t, a, b)

	mkApp := func(refs []map[string]interface{}) *v1beta1.Application {
		props, _ := json.Marshal(map[string]interface{}{
			"chart":      map[string]interface{}{"source": "foo"},
			"release":    map[string]interface{}{"namespace": "ns1"},
			"valuesFrom": refs,
		})
		return &v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "ns1"},
			Spec: v1beta1.ApplicationSpec{
				Components: []common.ApplicationComponent{
					{Name: "x", Type: "helmchart", Properties: &runtime.RawExtension{Raw: props}},
				},
			},
		}
	}

	abApp := mkApp([]map[string]interface{}{
		{"kind": "ConfigMap", "name": "a"},
		{"kind": "ConfigMap", "name": "b"},
	})
	baApp := mkApp([]map[string]interface{}{
		{"kind": "ConfigMap", "name": "b"},
		{"kind": "ConfigMap", "name": "a"},
	})

	abFp, _ := computeValuesFromContentFingerprint(context.Background(), abApp)
	baFp, _ := computeValuesFromContentFingerprint(context.Background(), baApp)
	if abFp == "" || abFp != baFp {
		t.Fatalf("expected order-independent aggregation, got %q vs %q", abFp, baFp)
	}
}

func TestComputeValuesFromContentFingerprint_CustomKey(t *testing.T) {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "cfg", Namespace: "ns1"},
		Data: map[string]string{
			"dev.yaml":  "replicas: 1\n",
			"prod.yaml": "replicas: 5\n",
		},
	}
	setupFakeClient(t, cm)

	devApp := newAppWithCMValuesFrom(t, "a", "ns1", "cfg", "dev.yaml")
	prodApp := newAppWithCMValuesFrom(t, "a", "ns1", "cfg", "prod.yaml")

	devFp, _ := computeValuesFromContentFingerprint(context.Background(), devApp)
	prodFp, _ := computeValuesFromContentFingerprint(context.Background(), prodApp)
	if devFp == prodFp {
		t.Fatalf("expected different keys to produce different fingerprints, both = %q", devFp)
	}
}

func TestComputeValuesFromContentFingerprint_BinaryDataRejected(t *testing.T) {
	// kubectl create cm --from-file with non-UTF-8 content writes the key into
	// .binaryData, not .data. valuesFrom requires textual YAML; the loader must
	// reject explicitly with a clear message rather than fall through to the
	// generic "key not found" error.
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "cfg", Namespace: "ns1"},
		BinaryData: map[string][]byte{"values.yaml": {0xff, 0xfe, 0x00}},
	}
	setupFakeClient(t, cm)
	app := newAppWithCMValuesFrom(t, "app1", "ns1", "cfg", "")
	_, err := computeValuesFromContentFingerprint(context.Background(), app)
	if err == nil {
		t.Fatalf("expected an explicit binaryData rejection error, got nil")
	}
	if !strings.Contains(err.Error(), "binaryData") {
		t.Fatalf("error should mention binaryData; got: %v", err)
	}
}

func TestComputeValuesFromContentFingerprint_RespectsContextCancellation(t *testing.T) {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "cfg", Namespace: "ns1"},
		Data:       map[string]string{"values.yaml": "replicas: 3\n"},
	}
	setupFakeClient(t, cm)
	app := newAppWithCMValuesFrom(t, "app1", "ns1", "cfg", "")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := computeValuesFromContentFingerprint(ctx, app)
	if err == nil {
		t.Fatalf("expected context.Canceled, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got: %v", err)
	}
}

func TestComputeValuesFromContentFingerprint_NoValuesFromHelmchartIsBackwardsCompat(t *testing.T) {
	// Backwards-compat regression guard: a helmchart Application that doesn't
	// declare valuesFrom must produce an empty fingerprint so workflow.go's
	// gate behaves identically to before this feature.
	app := &v1beta1.Application{
		Spec: v1beta1.ApplicationSpec{
			Components: []common.ApplicationComponent{
				{
					Name: "x",
					Type: "helmchart",
					Properties: &runtime.RawExtension{
						Raw: []byte(`{"chart":{"source":"foo"},"release":{"namespace":"ns1"},"values":{"replicaCount":3}}`),
					},
				},
			},
		},
	}
	got, err := computeValuesFromContentFingerprint(context.Background(), app)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "" {
		t.Fatalf("expected empty fingerprint for helmchart without valuesFrom, got %q", got)
	}
}

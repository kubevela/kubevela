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

package dryrun

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilfeature "k8s.io/apiserver/pkg/util/feature"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/features"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/sources"
)

// Reads are validated against the whole Application before any slice of it is
// rendered, so a bad one is still reported as itself.
func TestDryRunValidatesComponentReadsFirst(t *testing.T) {
	enableExpressions(t)
	app := &v1beta1.Application{Spec: v1beta1.ApplicationSpec{Components: []common.ApplicationComponent{
		{Name: "b", Type: "k8s-objects", Properties: &runtime.RawExtension{
			Raw: []byte(`{"v":"$(component.a.cluster(\"local\").output.data.v)"}`)}},
	}}}
	app.Name = "g"
	app.Annotations = map[string]string{oam.AnnotationCelExpressions: "true"}
	_, _, err := (&Option{}).ExecuteDryRun(context.Background(), app)
	require.ErrorContains(t, err, `the application has no component "a"`)
}

// The output says which values are stand-ins, since a non-text read is
// unchecked and a definition's own default can show where it would land.
func TestDryRunNoticesComponentReads(t *testing.T) {
	enableExpressions(t)
	app := &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{oam.AnnotationCelExpressions: "true"}},
		Spec: v1beta1.ApplicationSpec{Components: []common.ApplicationComponent{
			{Name: "a", Type: "k8s-objects"},
			{Name: "b", Type: "k8s-objects", Properties: &runtime.RawExtension{
				Raw: []byte(`{"v":"$(component.a.cluster(\"local\").output.data.v)"}`)},
				Traits: []common.ApplicationTrait{{Type: "scaler", Properties: &runtime.RawExtension{
					Raw: []byte(`{"replicas":"$(int(component.a.output.data.n))"}`)}}}},
		}}}
	var buff bytes.Buffer
	require.NoError(t, writeComponentReadsNotice(&buff, app))
	out := buff.String()
	require.Contains(t, out, "# component b reads component.a.cluster(\"local\").output.data.v")
	require.Contains(t, out, "# component b reads component.a.output.data.n")
	require.Contains(t, out, "may show the definition's default")

	buff.Reset()
	require.NoError(t, writeComponentReadsNotice(&buff, &v1beta1.Application{}))
	require.Empty(t, buff.String())
}

// The notice says how the controller will order each read.
func TestDryRunNoticesReadOrder(t *testing.T) {
	enableExpressions(t)
	app := &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{oam.AnnotationCelExpressions: "true"}},
		Spec: v1beta1.ApplicationSpec{Components: []common.ApplicationComponent{
			{Name: "db", Type: "k8s-objects"},
			{Name: "api", Type: "k8s-objects", Properties: &runtime.RawExtension{
				Raw: []byte(`{"v":"$(component.db.output.data.v)","f":"$(component.flags.cluster(\"east\").output.data.v)",` +
					`"q":"$(component.q.cluster(\"east\").namespace(\"infra\").output.data.v)",` +
					`"o":"$(component.ops.namespace(\"infra\").output.data.v)"}`)}},
		}}}
	var buff bytes.Buffer
	require.NoError(t, writeComponentReadsNotice(&buff, app))
	require.Contains(t, buff.String(), `# How they are ordered:
#   api dependsOn db, beside it
#   api reads flags in east: the workflow orders it
#   api reads ops in namespace infra: the workflow orders it
#   api reads q in east/infra: the workflow orders it`)
}

// Without the opt-in, $( ) is ordinary text: nothing is validated or noticed.
func TestDryRunLeavesAnApplicationThatHasNotOptedIn(t *testing.T) {
	enableExpressions(t)
	app := &v1beta1.Application{Spec: v1beta1.ApplicationSpec{Components: []common.ApplicationComponent{
		{Name: "b", Type: "k8s-objects", Properties: &runtime.RawExtension{Raw: []byte(`{"cmd":"$(SVC_HOST)"}`)}},
	}}}
	require.NoError(t, validateComponentReads(app))
	var buff bytes.Buffer
	require.NoError(t, writeComponentReadsNotice(&buff, app))
	require.Empty(t, buff.String())
}

func enableExpressions(t *testing.T) {
	t.Helper()
	before := map[string]bool{
		string(features.EnableCelExpressions):      utilfeature.DefaultMutableFeatureGate.Enabled(features.EnableCelExpressions),
		string(features.RequireCelExpressionOptIn): utilfeature.DefaultMutableFeatureGate.Enabled(features.RequireCelExpressionOptIn),
	}
	require.NoError(t, utilfeature.DefaultMutableFeatureGate.SetFromMap(map[string]bool{
		string(features.EnableCelExpressions): true, string(features.RequireCelExpressionOptIn): true}))
	t.Cleanup(func() { require.NoError(t, utilfeature.DefaultMutableFeatureGate.SetFromMap(before)) })
}

// Only an Application that reads another component renders in placeholder
// mode. Anything else renders exactly as before, so a definition that leaves a
// field incomplete still fails its dry-run rather than having it pruned.
func TestPlaceholdersOnlyForComponentReads(t *testing.T) {
	enableExpressions(t)
	optedIn := map[string]string{oam.AnnotationCelExpressions: "true"}
	reads := common.ApplicationComponent{Name: "b", Type: "k8s-objects", Properties: &runtime.RawExtension{
		Raw: []byte(`{"v":"$(component.a.output.data.v)"}`)}}
	plain := common.ApplicationComponent{Name: "b", Type: "k8s-objects", Properties: &runtime.RawExtension{
		Raw: []byte(`{"v":"$(context.appName)"}`)}}

	for name, tc := range map[string]struct {
		annotations map[string]string
		comp        common.ApplicationComponent
		want        bool
	}{
		"reads a component":  {optedIn, reads, true},
		"reads no component": {optedIn, plain, false},
		"has not opted in":   {nil, reads, false},
	} {
		t.Run(name, func(t *testing.T) {
			app := &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Annotations: tc.annotations},
				Spec: v1beta1.ApplicationSpec{Components: []common.ApplicationComponent{tc.comp}}}
			require.Equal(t, tc.want, sources.ComponentPlaceholders(withPlaceholders(context.Background(), app)))
		})
	}
}

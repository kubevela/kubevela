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

package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	velacommon "github.com/oam-dev/kubevela/pkg/utils/common"
	cmdutil "github.com/oam-dev/kubevela/pkg/utils/util"
)

func TestFormatAutoUpdate(t *testing.T) {
	r := require.New(t)
	yes, no := true, false
	// Nil and false must not render alike: an Application reconciled before the
	// field existed reports neither, and showing that as "false" would assert
	// something the controller never said.
	r.Equal("-", formatAutoUpdate(nil))
	r.Equal("true", formatAutoUpdate(&yes))
	r.Equal("false", formatAutoUpdate(&no))
}

func TestFormatValueUnquotesScalarsButNotCollections(t *testing.T) {
	r := require.New(t)
	raw := func(s string) *runtime.RawExtension { return &runtime.RawExtension{Raw: []byte(s)} }

	// A quoted string in a table column is noise; a collection keeps its braces
	// so it is obvious the whole thing substituted.
	r.Equal("nginx:1.27", formatValue(raw(`"nginx:1.27"`)))
	r.Equal("[8080,8443]", formatValue(raw(`[8080,8443]`)))
	r.Equal(`{"team":"platform"}`, formatValue(raw(`{"team":"platform"}`)))
	r.Equal("5432", formatValue(raw(`5432`)))
	r.Equal("-", formatValue(nil))
}

func TestFormatReaderAndPlacement(t *testing.T) {
	r := require.New(t)
	r.Equal("component/web (webservice)", formatReader(common.SourceConsumer{
		DefinitionKind: "component", Name: "web", Type: "webservice"}))
	// A workflow step has no placement, so it must not render a stray separator.
	r.Equal("workflowstep/notify", formatReader(common.SourceConsumer{
		DefinitionKind: "workflowstep", Name: "notify"}))
	// A placed reader always carries a cluster name, so an empty one means the
	// reader is not placed - a workflow step - rather than running locally.
	r.Equal("local", formatCluster("local"))
	r.Equal("eu-west", formatCluster("eu-west"))
	r.Equal("-", formatCluster(""))
}

func TestConsumerFilterComposesWithTheExistingFlags(t *testing.T) {
	r := require.New(t)
	web := common.SourceConsumer{Name: "web", Cluster: "local"}
	db := common.SourceConsumer{Name: "db", Cluster: "remote"}

	r.True(Filter{}.matchConsumer(web), "no filter matches everything")
	r.True(Filter{Component: "web"}.matchConsumer(web))
	r.False(Filter{Component: "web"}.matchConsumer(db))
	r.True(Filter{Cluster: "remote"}.matchConsumer(db))
	r.False(Filter{Cluster: "remote"}.matchConsumer(web))
}

func sourcesFixture() *v1beta1.Application {
	yes := true
	return &v1beta1.Application{
		ObjectMeta: metav1.ObjectMeta{Name: "checkout", Namespace: "prod"},
		Spec:       v1beta1.ApplicationSpec{Sources: []v1beta1.ApplicationSource{{Name: "registry"}}},
		Status: common.AppStatus{Sources: []common.ApplicationSourceStatus{{
			Name: "registry", Type: "configmap@v2", Phase: "Resolved", AutoUpdate: &yes,
			Resolutions: []common.SourceResolution{{StorageKey: "cm-local-a1", Clusters: []string{"local"}, Phase: "Resolved"}},
			ConsumedBy: []common.SourceConsumer{
				{DefinitionKind: "component", Name: "web", Cluster: "local", Namespace: "prod",
					Values: []common.SourceValue{{Property: "image", SourceAttr: "data.image",
						Value: &runtime.RawExtension{Raw: []byte(`"nginx:1.27"`)}}}},
				{DefinitionKind: "component", Name: "api", Cluster: "eu-west", Namespace: "prod",
					Values: []common.SourceValue{{Property: "image", SourceAttr: "data.image",
						Value: &runtime.RawExtension{Raw: []byte(`"nginx:1.27"`)}}}},
			},
		}}},
	}
}

// The table is for reading; -o is for scripting. Without it the only way to get
// at this is scraping column output, which is exactly what a stable format
// exists to avoid.
func TestPrintAppSourcesMachineReadable(t *testing.T) {
	r := require.New(t)
	cli := fake.NewClientBuilder().WithScheme(velacommon.Scheme).WithObjects(sourcesFixture()).Build()

	var buf bytes.Buffer
	r.NoError(printAppSources(cli, "prod", "checkout", Filter{}, "json", &buf))
	var got sourcesOutput
	r.NoError(json.Unmarshal(buf.Bytes(), &got))
	r.Equal("checkout", got.Name)
	r.Equal("prod", got.Namespace)
	r.Len(got.Sources, 1)
	r.Equal("registry", got.Sources[0].Name)
	r.Len(got.Sources[0].ConsumedBy, 2)

	buf.Reset()
	r.NoError(printAppSources(cli, "prod", "checkout", Filter{}, "yaml", &buf))
	r.Contains(buf.String(), "sourceAttr: data.image")
	r.NotContains(buf.String(), "+---", "yaml output must not carry table decoration")
}

// A filter that narrowed the table but not the machine-readable form would be a
// trap for anything scripting against it.
func TestPrintAppSourcesFiltersMachineReadableToo(t *testing.T) {
	r := require.New(t)
	cli := fake.NewClientBuilder().WithScheme(velacommon.Scheme).WithObjects(sourcesFixture()).Build()

	var buf bytes.Buffer
	r.NoError(printAppSources(cli, "prod", "checkout",
		Filter{Cluster: "eu-west"}, "json", &buf))
	var got sourcesOutput
	r.NoError(json.Unmarshal(buf.Bytes(), &got))
	r.Len(got.Sources, 1, "the binding is still reported: whether it resolved does not depend on the filter")
	r.Len(got.Sources[0].ConsumedBy, 1)
	r.Equal("api", got.Sources[0].ConsumedBy[0].Name)
}

func TestPrintAppSourcesJSONPath(t *testing.T) {
	r := require.New(t)
	cli := fake.NewClientBuilder().WithScheme(velacommon.Scheme).WithObjects(sourcesFixture()).Build()
	var buf bytes.Buffer
	r.NoError(printAppSources(cli, "prod", "checkout", Filter{},
		"jsonpath={.sources[0].phase}", &buf))
	r.Equal("Resolved", buf.String())
}

func TestSourceIndicatorVocabulary(t *testing.T) {
	r := require.New(t)
	r.Equal(emojiSucceed, sourceIndicator("Resolved"))
	r.Equal(emojiFail, sourceIndicator("Failed"))
	r.Equal(emojiSkip, sourceIndicator("Unused"))
	// Stale is not a failure. The Application works; its data has stopped moving,
	// and a cross would say something untrue.
	r.Equal(emojiExecuting, sourceIndicator("Stale"))
	// An unknown phase from a newer controller reads as in-progress rather than
	// as success, which is the safe direction.
	r.Equal(emojiExecuting, sourceIndicator("SomethingNew"))
	r.Equal(emojiExecuting, sourceIndicator(""))
}

func consumer(kind, name, cluster string) common.SourceConsumer {
	return common.SourceConsumer{DefinitionKind: kind, Name: name, Cluster: cluster}
}

// One component placed in three clusters is one reader that runs in three
// places, not three consumers.
func TestDistinctReadersDeduplicatesPlacements(t *testing.T) {
	r := require.New(t)
	src := common.ApplicationSourceStatus{ConsumedBy: []common.SourceConsumer{
		consumer("component", "web", "eu-west"),
		consumer("component", "web", "us-east"),
		consumer("trait", "web/ingress", "eu-west"),
	}}
	got := distinctReaders(src)
	r.Len(got, 2)
	r.Equal("web", got[0].Name)
	r.Equal("component", got[0].DefinitionKind)
}

func TestPrintSourcesOverviewShape(t *testing.T) {
	r := require.New(t)
	app := &v1beta1.Application{
		ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "prod"},
		Spec: v1beta1.ApplicationSpec{Sources: []v1beta1.ApplicationSource{
			{Name: "registry", Type: "configmap"}, {Name: "pending", Type: "atlas"},
		}},
		Status: common.AppStatus{Sources: []common.ApplicationSourceStatus{
			{Name: "registry", Type: "configmap", Phase: "Failed",
				Resolutions: []common.SourceResolution{
					{StorageKey: "configmap-eu-a1", Phase: "Resolved"},
					{StorageKey: "configmap-us-b2", Phase: "Failed", Message: "dial tcp: i/o timeout"},
				},
				ConsumedBy: []common.SourceConsumer{
					{DefinitionKind: "component", Name: "api", Type: "webservice", Cluster: "eu-west"},
				}},
		}},
	}
	var buf bytes.Buffer
	printSourcesOverview(cmdutil.IOStreams{Out: &buf, ErrOut: &buf}, app)
	out := buf.String()

	r.Contains(out, "  - Name: registry")
	r.Contains(out, "    Type: configmap")
	r.Contains(out, "    Instances:")
	r.Contains(out, "configmap-us-b2")
	// A failing instance says why, the way a component's Health does above it.
	r.Contains(out, "dial tcp: i/o timeout")
	r.Contains(out, "    Consumers:")
	r.Contains(out, "        Type: webservice (component)")
	// Declared but not yet resolved still appears, as in-progress.
	r.Contains(out, "  - Name: pending")
	r.Contains(out, emojiExecuting)

	buf.Reset()
	printSourcesOverview(cmdutil.IOStreams{Out: &buf, ErrOut: &buf},
		&v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: "b"}})
	r.Empty(buf.String(), "an Application with no sources says nothing at all")
}

func TestPrintSourcesOverviewTruncatesConsumers(t *testing.T) {
	r := require.New(t)
	src := common.ApplicationSourceStatus{Name: "cfg", Type: "configmap", Phase: "Resolved"}
	for _, n := range []string{"a", "b", "c", "d", "e", "f", "g"} {
		src.ConsumedBy = append(src.ConsumedBy, consumer("component", n, "local"))
	}
	app := &v1beta1.Application{
		ObjectMeta: metav1.ObjectMeta{Name: "a"},
		Spec:       v1beta1.ApplicationSpec{Sources: []v1beta1.ApplicationSource{{Name: "cfg"}}},
		Status:     common.AppStatus{Sources: []common.ApplicationSourceStatus{src}},
	}
	var buf bytes.Buffer
	printSourcesOverview(cmdutil.IOStreams{Out: &buf, ErrOut: &buf}, app)
	r.Contains(buf.String(), "... and 2 more")
	r.NotContains(buf.String(), "Name: g")
}

func TestFormatConsumerType(t *testing.T) {
	r := require.New(t)
	r.Equal("webservice (component)", formatConsumerType(
		common.SourceConsumer{DefinitionKind: "component", Type: "webservice"}))
	r.Equal("notification (workflowstep)", formatConsumerType(
		common.SourceConsumer{DefinitionKind: "workflowstep", Type: "notification"}))
	// A reader whose type is unset is still a component, and that half is worth
	// keeping - better than an empty parenthetical.
	r.Equal("component", formatConsumerType(common.SourceConsumer{DefinitionKind: "component"}))
}

// The table is what an operator actually reads, and it was the half of this
// command with no test at all.
func TestPrintAppSourcesTable(t *testing.T) {
	r := require.New(t)
	cli := fake.NewClientBuilder().WithScheme(velacommon.Scheme).WithObjects(sourcesFixture()).Build()

	var buf bytes.Buffer
	r.NoError(printAppSources(cli, "prod", "checkout", Filter{}, "", &buf))
	out := buf.String()

	r.Contains(out, "Sources of prod/checkout")
	for _, col := range []string{"NAME", "TYPE", "PHASE", "AUTO-UPDATE", "CLUSTERS", "EXPIRES", "STORAGE KEY"} {
		r.Contains(out, col)
	}
	r.Contains(out, "registry")
	r.Contains(out, "cm-local-a1")
	r.Contains(out, "Consumed by:")
	for _, col := range []string{"SOURCE", "READER", "CLUSTER", "NAMESPACE", "PROPERTY", "SOURCE ATTR", "VALUE"} {
		r.Contains(out, col)
	}
	r.Contains(out, "nginx:1.27", "a scalar value is shown unquoted")
	r.Contains(out, "eu-west", "each placement is its own row")
}

// A binding whose key varies has an entry per key, and the binding's own columns
// are printed once so it reads as one thing with several entries.
func TestPrintAppSourcesTableGroupsResolutions(t *testing.T) {
	r := require.New(t)
	app := sourcesFixture()
	app.Status.Sources[0].Resolutions = append(app.Status.Sources[0].Resolutions,
		common.SourceResolution{StorageKey: "cm-eu-b2", Clusters: []string{"eu-west"}, Phase: "Failed",
			Message: "dial tcp: i/o timeout"})
	cli := fake.NewClientBuilder().WithScheme(velacommon.Scheme).WithObjects(app).Build()

	var buf bytes.Buffer
	r.NoError(printAppSources(cli, "prod", "checkout", Filter{}, "", &buf))
	out := buf.String()

	r.Contains(out, "cm-local-a1")
	r.Contains(out, "cm-eu-b2")
	r.Equal(1, strings.Count(out, "configmap@v2"),
		"the binding's own columns are printed once, against its first entry")
	r.Contains(out, "dial tcp: i/o timeout",
		"a resolution message is printed in full below the table, not truncated into a column")
}

// A message is the only place a false auto-update says which of the gate, the
// binding and a publishVersion pin won, so it must not be lost.
func TestPrintAppSourcesTablePrintsTheBindingMessage(t *testing.T) {
	r := require.New(t)
	app := sourcesFixture()
	app.Status.Sources[0].Message = "auto-update is off: pinned by publishVersion"
	cli := fake.NewClientBuilder().WithScheme(velacommon.Scheme).WithObjects(app).Build()

	var buf bytes.Buffer
	r.NoError(printAppSources(cli, "prod", "checkout", Filter{}, "", &buf))
	r.Contains(buf.String(), "pinned by publishVersion")
}

// Three different nothings, each with its own sentence. "No sources" and "not
// resolved yet" are not the same situation and an operator acts differently on
// them.
func TestPrintAppSourcesDistinguishesTheEmptyCases(t *testing.T) {
	r := require.New(t)
	build := func(app *v1beta1.Application) string {
		cli := fake.NewClientBuilder().WithScheme(velacommon.Scheme).WithObjects(app).Build()
		var buf bytes.Buffer
		r.NoError(printAppSources(cli, "prod", "checkout", Filter{}, "", &buf))
		return buf.String()
	}

	none := &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: "checkout", Namespace: "prod"}}
	r.Contains(build(none), "declares no sources")

	unresolved := &v1beta1.Application{
		ObjectMeta: metav1.ObjectMeta{Name: "checkout", Namespace: "prod"},
		Spec:       v1beta1.ApplicationSpec{Sources: []v1beta1.ApplicationSource{{Name: "registry"}}},
	}
	r.Contains(build(unresolved), "has not resolved them yet")

	unread := sourcesFixture()
	unread.Status.Sources[0].ConsumedBy = nil
	r.Contains(build(unread), "nothing has consumed a source value")
}

// A binding with no resolutions still gets a row: it is declared, and silence
// would read as though it did not exist.
func TestPrintAppSourcesTableShowsABindingWithNoEntries(t *testing.T) {
	r := require.New(t)
	app := sourcesFixture()
	app.Status.Sources[0].Resolutions = nil
	cli := fake.NewClientBuilder().WithScheme(velacommon.Scheme).WithObjects(app).Build()

	var buf bytes.Buffer
	r.NoError(printAppSources(cli, "prod", "checkout", Filter{}, "", &buf))
	r.Contains(buf.String(), "registry")
}

func TestPrintAppSourcesReportsALookupFailure(t *testing.T) {
	cli := fake.NewClientBuilder().WithScheme(velacommon.Scheme).Build()
	var buf bytes.Buffer
	require.Error(t, printAppSources(cli, "prod", "absent", Filter{}, "", &buf))
}

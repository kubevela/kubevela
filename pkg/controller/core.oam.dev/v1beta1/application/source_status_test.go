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
	"testing"

	"github.com/oam-dev/kubevela/pkg/sources"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
)

func handlerFor(annotations map[string]string, sources ...v1beta1.ApplicationSource) *AppHandler {
	return &AppHandler{app: &v1beta1.Application{
		ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "default", Annotations: annotations},
		Spec:       v1beta1.ApplicationSpec{Sources: sources},
	}}
}

// The Application-level list answers "did my data arrive", once per binding
// rather than once per binding per component.
func TestSourceStatusListReportsEveryBindingOnce(t *testing.T) {
	r := require.New(t)
	h := handlerFor(nil,
		v1beta1.ApplicationSource{Name: "registry", Type: "configmap"},
		v1beta1.ApplicationSource{Name: "unread", Type: "configmap"},
	)

	// Two components read the same binding. That is one binding, two consumers.
	for _, comp := range []string{"web", "api"} {
		h.recordSourceResolution(sourceKindComponent, comp, "webservice", "local", "default",
			map[string]sources.SourceResolutionStatus{
				"registry": {
					Name: "registry", Type: "configmap", Phase: sourcePhaseResolved,
					Config: "configmap-local-default-abc", ExpiresAt: "2026-08-19T15:00:00Z",
					ConsumedFields: map[string]interface{}{"data.image": "nginx:1.27"},
				},
			})
	}

	out := h.sourceStatusList()
	r.Len(out, 2, "one row per declared binding, in spec order")
	r.Equal("registry", out[0].Name)
	r.Equal(sourcePhaseResolved, out[0].Phase)
	r.Len(out[0].Resolutions, 1, "both readers hit the same cache entry, so it is one resolution")
	r.Equal("configmap-local-default-abc", out[0].Resolutions[0].StorageKey)
	r.Len(out[0].ConsumedBy, 2, "both readers recorded against the one binding")
	r.Equal(sourceKindComponent, out[0].ConsumedBy[0].DefinitionKind)
	r.Equal("webservice", out[0].ConsumedBy[0].Type)

	// A binding nothing read is reported as Unused rather than omitted; silence
	// would be ambiguous with a failure.
	r.Equal("unread", out[1].Name)
	r.Equal(sourcePhaseUnused, out[1].Phase)
	r.Empty(out[1].ConsumedBy)
}

// A workflow step has no status of its own to carry this, so it has to land here.
func TestSourceStatusListRecordsNonComponentReaders(t *testing.T) {
	r := require.New(t)
	h := handlerFor(nil, v1beta1.ApplicationSource{Name: "registry", Type: "configmap"})

	h.recordSourceResolution(sourceKindWorkflowStep, "notify", "notification", "", "",
		map[string]sources.SourceResolutionStatus{
			"registry": {Name: "registry", Phase: sourcePhaseResolved,
				ConsumedFields: map[string]interface{}{"data.channel": "#deploys"}},
		})

	out := h.sourceStatusList()
	r.Len(out[0].ConsumedBy, 1)
	r.Equal(sourceKindWorkflowStep, out[0].ConsumedBy[0].DefinitionKind)
	r.Equal("notify", out[0].ConsumedBy[0].Name)
	r.Empty(out[0].ConsumedBy[0].Cluster, "a workflow step is not placed in a cluster")
}

// A failure seen by any reader is the one worth surfacing, even if another
// reader resolved the same binding happily from cache.
func TestSourceStatusListPrefersAFailure(t *testing.T) {
	r := require.New(t)
	h := handlerFor(nil, v1beta1.ApplicationSource{Name: "registry", Type: "configmap"})

	h.recordSourceResolution(sourceKindComponent, "web", "webservice", "local", "default",
		map[string]sources.SourceResolutionStatus{
			"registry": {Name: "registry", Phase: sourcePhaseResolved,
				ConsumedFields: map[string]interface{}{"data.image": "nginx"}},
		})
	h.recordSourceResolution(sourceKindComponent, "api", "webservice", "remote", "default",
		map[string]sources.SourceResolutionStatus{
			"registry": {Name: "registry", Phase: sourcePhaseFailed, Message: "fetch timed out",
				ConsumedFields: map[string]interface{}{"data.image": "nginx"}},
		})

	out := h.sourceStatusList()
	r.Equal(sourcePhaseFailed, out[0].Phase, "a failure anywhere is the binding's phase")
	// The reason belongs to the entry that failed. The binding's own Message is
	// for things about the binding, like why autoUpdate did not take effect.
	r.Empty(out[0].Message)
	r.Len(out[0].Resolutions, 1)
	r.Equal("fetch timed out", out[0].Resolutions[0].Message)
}

func TestSourceStatusAutoUpdateIsResolvedNotDeclared(t *testing.T) {
	r := require.New(t)
	yes, no := true, false

	on := handlerFor(nil, v1beta1.ApplicationSource{Name: "a", AutoUpdate: &yes}).sourceStatusList()
	r.NotNil(on[0].AutoUpdate)
	r.True(*on[0].AutoUpdate)

	off := handlerFor(nil, v1beta1.ApplicationSource{Name: "a", AutoUpdate: &no}).sourceStatusList()
	r.False(*off[0].AutoUpdate)

	// A pin beats the binding. The bool says false; the message says why, since a
	// bool alone cannot distinguish pinned from opted-out from gate-off.
	pinned := handlerFor(map[string]string{oam.AnnotationPublishVersion: "v1"},
		v1beta1.ApplicationSource{Name: "a", AutoUpdate: &yes}).sourceStatusList()
	r.False(*pinned[0].AutoUpdate)
	r.Contains(pinned[0].Message, "publishVersion")
}

// A read has to say where the value went, not just what was read. Once a
// property is assembled from more than one source, "which field did I read" on
// its own cannot be mapped back to anything.
func TestConsumedReadsCarryTheDestinationProperty(t *testing.T) {
	r := require.New(t)
	h := handlerFor(nil, v1beta1.ApplicationSource{Name: "db", Type: "dbinfo"})

	h.recordSourceResolution(sourceKindComponent, "web", "webservice", "local", "default",
		map[string]sources.SourceResolutionStatus{
			"db": {
				Name:  "db",
				Phase: sourcePhaseResolved,
				// host is assembled from two fields; image from one.
				ConsumedFields: map[string]interface{}{"addr": "db.internal", "port": 5432, "img": "pg:16"},
				Reads: []sources.SourceRead{
					{SourceAttr: "addr", Property: "host", Value: "db.internal"},
					{SourceAttr: "port", Property: "host", Value: 5432},
					{SourceAttr: "img", Property: "image", Value: "pg:16"},
				},
			},
		})

	values := h.sourceStatusList()[0].ConsumedBy[0].Values
	r.Len(values, 3)
	// Sorted by property then field, so the report is stable across reconciles
	// rather than following Go's map iteration order.
	r.Equal("host", values[0].Property)
	r.Equal("addr", values[0].SourceAttr)
	r.Equal("host", values[1].Property)
	r.Equal("port", values[1].SourceAttr)
	r.Equal("image", values[2].Property)
	r.Equal("img", values[2].SourceAttr)
	r.JSONEq(`"db.internal"`, string(values[0].Value.Raw))
	r.JSONEq(`5432`, string(values[1].Value.Raw))
}

// A chained source reads on its own behalf. Attributing those reads to whichever
// component triggered the chain would hide the chain and misreport the component.
func TestChainedSourceReadsAreNotClaimedByTheComponent(t *testing.T) {
	r := require.New(t)
	h := handlerFor(nil, v1beta1.ApplicationSource{Name: "atlas", Type: "atlas"})

	h.recordSourceResolution(sourceKindComponent, "web", "webservice", "local", "default",
		map[string]sources.SourceResolutionStatus{
			"atlas": {
				Name:           "atlas",
				Phase:          sourcePhaseResolved,
				ConsumedFields: map[string]interface{}{"clusterName": "eu-west-1"},
				Reads: []sources.SourceRead{
					// read by the chained source "config", not by the component
					{SourceAttr: "clusterName", Property: "path", Value: "eu-west-1",
						ReaderKind: "source", ReaderName: "config"},
				},
			},
		})

	r.Empty(h.sourceStatusList()[0].ConsumedBy[0].Values,
		"the component made no reads of its own, so it must claim none")
}

// A sensitive field must stay redacted when the read is the struct above it.
// Marks are schema paths - "db.password" - and an expression may substitute a
// whole collection, so a read of "db" carries the password with it. Checking
// only whether the read sits at or below a mark misses that entirely and writes
// the secret into a status anyone with get on Applications can read.
func TestSensitiveValuesSurviveAWholeStructRead(t *testing.T) {
	r := require.New(t)
	h := handlerFor(nil, v1beta1.ApplicationSource{Name: "creds", Type: "dbcreds"})

	h.recordSourceResolution(sourceKindComponent, "web", "webservice", "local", "default",
		map[string]sources.SourceResolutionStatus{
			"creds": {
				Name: "creds", Phase: sourcePhaseResolved,
				SensitivePaths: []string{"db.password", "members.token"},
				ConsumedFields: map[string]interface{}{"db": "x"},
				Reads: []sources.SourceRead{
					{SourceAttr: "db", Property: "settings", Value: map[string]interface{}{
						"host": "db.internal", "password": "hunter2",
					}},
					{SourceAttr: "members", Property: "team", Value: []interface{}{
						map[string]interface{}{"name": "ana", "token": "t-secret"},
					}},
				},
			},
		})

	values := h.sourceStatusList()[0].ConsumedBy[0].Values
	for _, rd := range values {
		raw := string(rd.Value.Raw)
		r.NotContains(raw, "hunter2", "a password under a read struct must not reach status")
		r.NotContains(raw, "t-secret", "a token inside a read list must not reach status either")
		r.Contains(raw, "***")
	}
	// Redaction is surgical: what was not marked still shows.
	r.Contains(string(values[0].Value.Raw)+string(values[1].Value.Raw), "db.internal")
	r.Contains(string(values[0].Value.Raw)+string(values[1].Value.Raw), "ana")
}

// The per-component list is gone, so a consumer entry has to place itself.
// Without the namespace an override policy putting one component in two
// namespaces of a cluster produces two entries that cannot be told apart.
func TestConsumerRecordsItsPlacement(t *testing.T) {
	r := require.New(t)
	h := handlerFor(nil, v1beta1.ApplicationSource{Name: "cfg", Type: "configmap"})

	for _, ns := range []string{"team-a", "team-b"} {
		h.recordSourceResolution(sourceKindComponent, "web", "webservice", "local", ns,
			map[string]sources.SourceResolutionStatus{
				"cfg": {Name: "cfg", Phase: sourcePhaseResolved,
					ConsumedFields: map[string]interface{}{"data.image": "nginx"},
					Reads: []sources.SourceRead{
						{SourceAttr: "data.image", Property: "image", Value: "nginx"},
					}},
			})
	}

	consumers := h.sourceStatusList()[0].ConsumedBy
	r.Len(consumers, 2)
	r.Equal("local", consumers[0].Cluster)
	r.ElementsMatch([]string{"team-a", "team-b"},
		[]string{consumers[0].Namespace, consumers[1].Namespace})
}

// A binding keyed on the cluster resolves separately in each, with its own cache
// entry and its own expiry. Collapsing them into one config and one expiry meant
// status named one of three, and named it by whichever cluster reconciled last.
func TestResolutionsAreKeyedByCacheEntry(t *testing.T) {
	r := require.New(t)
	h := handlerFor(nil, v1beta1.ApplicationSource{Name: "cfg", Type: "configmap"})

	for _, c := range []struct{ cluster, config, expires string }{
		{"eu-west", "cfg-eu-west-a1", "2026-08-19T16:00:00Z"},
		{"us-east", "cfg-us-east-b2", "2026-08-19T17:00:00Z"},
	} {
		h.recordSourceResolution(sourceKindComponent, "web", "webservice", c.cluster, "prod",
			map[string]sources.SourceResolutionStatus{
				"cfg": {Name: "cfg", Phase: sourcePhaseResolved, Config: c.config, ExpiresAt: c.expires,
					ConsumedFields: map[string]interface{}{"data.image": "nginx"}},
			})
	}

	got := h.sourceStatusList()[0]
	r.Len(got.Resolutions, 2, "two clusters, two cache entries, two expiries")
	r.ElementsMatch([]string{"cfg-eu-west-a1", "cfg-us-east-b2"},
		[]string{got.Resolutions[0].StorageKey, got.Resolutions[1].StorageKey})
	r.Equal([]string{"eu-west"}, got.Resolutions[0].Clusters)
}

// A source whose cache key ignores the cluster genuinely shares one entry, so
// keying resolutions by cluster would invent a second and report one expiry
// twice.
func TestASharedCacheEntryIsOneResolution(t *testing.T) {
	r := require.New(t)
	h := handlerFor(nil, v1beta1.ApplicationSource{Name: "cfg", Type: "git-file"})

	for _, cluster := range []string{"eu-west", "us-east"} {
		h.recordSourceResolution(sourceKindComponent, "web", "webservice", cluster, "prod",
			map[string]sources.SourceResolutionStatus{
				"cfg": {Name: "cfg", Phase: sourcePhaseResolved, Config: "git-file-shared",
					ConsumedFields: map[string]interface{}{"content": "x"}},
			})
	}

	got := h.sourceStatusList()[0]
	r.Len(got.Resolutions, 1)
	r.ElementsMatch([]string{"eu-west", "us-east"}, got.Resolutions[0].Clusters)
}

// The binding's own phase is the worst any cluster saw, so which cluster
// reconciled last cannot decide whether the Application looks healthy.
func TestBindingPhaseIsTheWorstAcrossClusters(t *testing.T) {
	r := require.New(t)
	h := handlerFor(nil, v1beta1.ApplicationSource{Name: "cfg", Type: "configmap"})

	h.recordSourceResolution(sourceKindComponent, "web", "webservice", "eu-west", "prod",
		map[string]sources.SourceResolutionStatus{
			"cfg": {Name: "cfg", Phase: sourcePhaseFailed, Config: "cfg-eu", Message: "i/o timeout"},
		})
	h.recordSourceResolution(sourceKindComponent, "web", "webservice", "us-east", "prod",
		map[string]sources.SourceResolutionStatus{
			"cfg": {Name: "cfg", Phase: sourcePhaseResolved, Config: "cfg-us"},
		})

	got := h.sourceStatusList()[0]
	r.Equal(sourcePhaseFailed, got.Phase, "one cluster failing is a failure")
	// ...and the failure stays attached to the entry that failed, not to the binding.
	for _, res := range got.Resolutions {
		if res.StorageKey == "cfg-eu" {
			r.Equal("i/o timeout", res.Message)
		} else {
			r.Empty(res.Message)
		}
	}
}

// Cluster is not the axis a resolution divides on. A source keyed on the
// component has an entry per component inside a single cluster, so a breakdown
// that listed clusters would print the same name twice and explain nothing.
func TestResolutionsDivideWithinOneCluster(t *testing.T) {
	r := require.New(t)
	h := handlerFor(nil, v1beta1.ApplicationSource{Name: "percomp", Type: "per-component"})

	for _, c := range []struct{ comp, key string }{
		{"web", "per-component-local-prod-web-a1"},
		{"api", "per-component-local-prod-api-b2"},
	} {
		h.recordSourceResolution(sourceKindComponent, c.comp, "webservice", "local", "prod",
			map[string]sources.SourceResolutionStatus{
				"percomp": {Name: "percomp", Phase: sourcePhaseResolved, Config: c.key,
					ConsumedFields: map[string]interface{}{"x": "y"}},
			})
	}

	got := h.sourceStatusList()[0]
	r.Len(got.Resolutions, 2, "two entries, one cluster")
	for _, res := range got.Resolutions {
		r.Equal([]string{"local"}, res.Clusters, "both served the same cluster")
	}
	r.NotEqual(got.Resolutions[0].StorageKey, got.Resolutions[1].StorageKey)
}

// TestSourceStatusListRecordsAReaderOnce pins that a component rendered more
// than once in a reconcile appears once in consumedBy.
//
// collectHealthStatus records the reads, and it runs both in the ordinary apply
// and again in refreshSourceDrivenComponents when a source value changed, so a
// component that auto-updates is recorded twice in the same reconcile.
func TestSourceStatusListRecordsAReaderOnce(t *testing.T) {
	r := require.New(t)
	h := handlerFor(nil, v1beta1.ApplicationSource{Name: "cfg", Type: "configmap"})

	rs := map[string]sources.SourceResolutionStatus{
		"cfg": {Name: "cfg", Phase: sourcePhaseResolved,
			ConsumedFields: map[string]interface{}{"data.image": "nginx"},
			Reads: []sources.SourceRead{
				{SourceAttr: "data.image", Property: "image", Value: "nginx"},
			}},
	}
	h.recordSourceResolution(sourceKindComponent, "web", "webservice", "local", "default", rs)
	h.recordSourceResolution(sourceKindComponent, "web", "webservice", "local", "default", rs)

	out := h.sourceStatusList()
	r.Len(out, 1)
	r.Len(out[0].ConsumedBy, 1, "one reader recorded twice is still one reader")
	r.Equal("web", out[0].ConsumedBy[0].Name)
	r.Len(out[0].ConsumedBy[0].Values, 1)
}

// The same component in two clusters is two readers, not one.
func TestSourceStatusListKeepsReadersPerPlacement(t *testing.T) {
	r := require.New(t)
	h := handlerFor(nil, v1beta1.ApplicationSource{Name: "cfg", Type: "configmap"})

	rs := map[string]sources.SourceResolutionStatus{
		"cfg": {Name: "cfg", Phase: sourcePhaseResolved,
			ConsumedFields: map[string]interface{}{"data.image": "nginx"}},
	}
	h.recordSourceResolution(sourceKindComponent, "web", "webservice", "local", "default", rs)
	h.recordSourceResolution(sourceKindComponent, "web", "webservice", "remote", "default", rs)

	out := h.sourceStatusList()
	r.Len(out[0].ConsumedBy, 2, "a component placed in two clusters read it twice, in two places")
}

// Asking for one field to be masked must not hide every other field.
//
// statusPolicy is a struct of independent knobs, so setting maskPaths says
// nothing about whether values are published - the two are separate requests,
// and the narrower one is the one being made here.
func TestMaskingOnePathKeepsTheRestVisible(t *testing.T) {
	r := require.New(t)
	h := handlerFor(nil, v1beta1.ApplicationSource{
		Name: "creds", Type: "dbcreds",
		StatusPolicy: &v1beta1.ApplicationSourceStatusPolicy{MaskPaths: []string{"password"}},
	})

	h.recordSourceResolution(sourceKindComponent, "web", "webservice", "local", "default",
		map[string]sources.SourceResolutionStatus{
			"creds": {
				Name: "creds", Phase: sourcePhaseResolved,
				ConsumedFields: map[string]interface{}{"host": "db.internal"},
				Reads: []sources.SourceRead{
					{SourceAttr: "host", Property: "settings", Value: "db.internal"},
					{SourceAttr: "password", Property: "secret", Value: "hunter2"},
				},
			},
		})

	values := h.sourceStatusList()[0].ConsumedBy[0].Values
	r.Len(values, 2, "masking one path must not drop the reads")
	joined := ""
	for _, v := range values {
		joined += string(v.Value.Raw)
	}
	r.Contains(joined, "db.internal", "an unmasked value stays visible")
	r.NotContains(joined, "hunter2", "the masked one does not")
}

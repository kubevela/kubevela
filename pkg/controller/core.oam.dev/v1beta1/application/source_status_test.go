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
	"errors"
	"testing"

	"github.com/oam-dev/kubevela/pkg/sources"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	featuregatetesting "k8s.io/component-base/featuregate/testing"

	workflowv1alpha1 "github.com/kubevela/workflow/api/v1alpha1"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/appfile"
	velaprocess "github.com/oam-dev/kubevela/pkg/cue/process"
	"github.com/oam-dev/kubevela/pkg/features"
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

// A step resolves its sources when it runs, so a reconcile in which it does not
// run records nothing for it. What it read stays reported for the rest of its run.
func TestSourceStatusKeepsWhatThisRunsStepsRead(t *testing.T) {
	r := require.New(t)
	previous := common.ApplicationSourceStatus{Name: "registry", Type: "configmap", Phase: sourcePhaseResolved,
		Resolutions: []common.SourceResolution{{StorageKey: "configmap-local-default-abc", ExpiresAt: "2026-10-14T19:00:00Z"}},
		ConsumedBy: []common.SourceConsumer{
			{DefinitionKind: sourceKindWorkflowStep, Name: "notify", Type: "notification",
				Values: []common.SourceValue{{SourceAttr: "data.channel", Property: "channel"}}},
			{DefinitionKind: sourceKindComponent, Name: "web", Type: "webservice", Cluster: "local"},
		}}
	withRun := func(steps ...string) *AppHandler {
		h := handlerFor(nil, v1beta1.ApplicationSource{Name: "registry", Type: "configmap"})
		h.app.Status.Sources = []common.ApplicationSourceStatus{previous}
		h.app.Status.Workflow = &common.WorkflowStatus{}
		for _, s := range steps {
			h.app.Status.Workflow.Steps = append(h.app.Status.Workflow.Steps,
				workflowv1alpha1.WorkflowStepStatus{StepStatus: workflowv1alpha1.StepStatus{Name: s}})
		}
		h.keepStepSourceReads()
		return h
	}

	out := withRun("notify").sourceStatusList()
	r.Equal(sourcePhaseResolved, out[0].Phase)
	r.Equal([]common.SourceConsumer{previous.ConsumedBy[0]}, out[0].ConsumedBy,
		"the step is kept; a component records itself on every render")
	r.Equal(previous.Resolutions, out[0].Resolutions, "and where its value came from")

	out = withRun().sourceStatusList()
	r.Equal(sourcePhaseUnused, out[0].Phase, "a new run has read nothing yet")
}

// What a step reads this reconcile replaces what the last report said, so a
// source that failed once and resolved on the retry is reported resolved.
func TestSourceStatusRecoversWhenAKeptStepReadsAgain(t *testing.T) {
	r := require.New(t)
	h := handlerFor(nil, v1beta1.ApplicationSource{Name: "registry", Type: "configmap"})
	h.app.Status.Sources = []common.ApplicationSourceStatus{{Name: "registry", Type: "configmap", Phase: sourcePhaseFailed,
		Resolutions: []common.SourceResolution{{Phase: sourcePhaseFailed, Message: "fetch timed out"}},
		ConsumedBy:  []common.SourceConsumer{{DefinitionKind: sourceKindWorkflowStep, Name: "notify", Type: "notification"}}}}
	h.app.Status.Workflow = &common.WorkflowStatus{Steps: []workflowv1alpha1.WorkflowStepStatus{
		{StepStatus: workflowv1alpha1.StepStatus{Name: "notify"}}}}
	h.keepStepSourceReads()

	h.recordSourceResolution(sourceKindWorkflowStep, "notify", "notification", "", "",
		map[string]sources.SourceResolutionStatus{"registry": {Name: "registry", Phase: sourcePhaseResolved,
			Config: "configmap-local-default-abc", ConsumedFields: map[string]interface{}{"data.channel": "#deploys"}}})

	out := h.sourceStatusList()
	r.Equal(sourcePhaseResolved, out[0].Phase)
	r.Len(out[0].Resolutions, 1)
	r.Equal("configmap-local-default-abc", out[0].Resolutions[0].StorageKey)
	r.Len(out[0].ConsumedBy, 1, "the step once, as it read this time")
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

	// A binding that opts in stays live under a pin.
	pin := map[string]string{oam.AnnotationPublishVersion: "v1"}
	optedIn := handlerFor(pin, v1beta1.ApplicationSource{Name: "a", AutoUpdate: &yes}).sourceStatusList()
	r.True(*optedIn[0].AutoUpdate)
	r.Empty(optedIn[0].Message)

	// One the default would have turned on is frozen by the pin. The bool says
	// false; the message says why and what keeps it live, since a bool alone
	// cannot distinguish pinned from opted-out from gate-off.
	featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.EnableSourceAutoUpdate, true)
	frozen := handlerFor(pin, v1beta1.ApplicationSource{Name: "a"}).sourceStatusList()
	r.False(*frozen[0].AutoUpdate)
	r.Contains(frozen[0].Message, "publishVersion")
	r.Contains(frozen[0].Message, "autoUpdate: true")
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

// A workflow step resolves its forEach.items and its properties separately, so
// what it read of one source arrives in two recordings, and the report holds both.
func TestAStepReadingOneSourceTwiceKeepsBothReads(t *testing.T) {
	r := require.New(t)
	h := handlerFor(nil, v1beta1.ApplicationSource{Name: "cfg", Type: "configmap"})
	reading := func(attr, property, value string) map[string]sources.SourceResolutionStatus {
		return map[string]sources.SourceResolutionStatus{"cfg": {Name: "cfg", Phase: sourcePhaseResolved,
			ConsumedFields: map[string]interface{}{attr: value},
			Reads:          []sources.SourceRead{{SourceAttr: attr, Property: property, Value: value}}}}
	}
	h.recordSourceResolution(sourceKindWorkflowStep, "rollout", "deploy", "", "", reading("data.regions", "forEach.items", "eu"))
	h.recordSourceResolution(sourceKindWorkflowStep, "rollout", "deploy", "", "", reading("data.channel", "channel", "#deploys"))

	out := h.sourceStatusList()
	r.Len(out[0].ConsumedBy, 1)
	read := []string{}
	for _, v := range out[0].ConsumedBy[0].Values {
		read = append(read, v.Property+"<-"+v.SourceAttr)
	}
	r.Equal([]string{"channel<-data.channel", "forEach.items<-data.regions"}, read)
}

// A kept step that did not read again keeps where its value came from, even
// when another reader resolved the same binding this reconcile.
func TestAKeptStepKeepsItsResolutionBesideALiveReader(t *testing.T) {
	r := require.New(t)
	h := handlerFor(nil, v1beta1.ApplicationSource{Name: "registry", Type: "configmap"})
	h.app.Status.Sources = []common.ApplicationSourceStatus{{Name: "registry", Type: "configmap", Phase: sourcePhaseResolved,
		Resolutions: []common.SourceResolution{{StorageKey: "configmap-step", ExpiresAt: "2026-10-14T19:00:00Z"}},
		ConsumedBy:  []common.SourceConsumer{{DefinitionKind: sourceKindWorkflowStep, Name: "notify", Type: "notification"}}}}
	h.app.Status.Workflow = &common.WorkflowStatus{Steps: []workflowv1alpha1.WorkflowStepStatus{
		{StepStatus: workflowv1alpha1.StepStatus{Name: "notify"}}}}
	h.keepStepSourceReads()

	h.recordSourceResolution(sourceKindComponent, "web", "webservice", "local", "default",
		map[string]sources.SourceResolutionStatus{"registry": {Name: "registry", Phase: sourcePhaseResolved,
			Config: "configmap-web", ConsumedFields: map[string]interface{}{"data.image": "nginx"}}})

	out := h.sourceStatusList()
	r.Len(out[0].ConsumedBy, 2)
	keys := []string{}
	for _, res := range out[0].Resolutions {
		keys = append(keys, res.StorageKey)
	}
	r.ElementsMatch([]string{"configmap-web", "configmap-step"}, keys)
	for _, res := range out[0].Resolutions {
		if res.StorageKey == "configmap-step" {
			r.Equal("2026-10-14T19:00:00Z", res.ExpiresAt)
		}
	}
}

// A value served past its TTL is reported as Stale. The resolver calls it
// resolved, because a value is in hand, and flags it; naming the phase is this
// layer's job, which is what the phase's own documentation says.
func TestSourceStatusListReportsStale(t *testing.T) {
	r := require.New(t)
	h := handlerFor(nil, v1beta1.ApplicationSource{Name: "registry", Type: "configmap"})

	h.recordSourceResolution(sourceKindComponent, "web", "webservice", "local", "default",
		map[string]sources.SourceResolutionStatus{
			"registry": {Name: "registry", Phase: sourcePhaseResolved, Stale: true,
				Message:        "refresh failed; serving stale cached value",
				ConsumedFields: map[string]interface{}{"data.image": "nginx"}},
		})

	out := h.sourceStatusList()
	r.Equal(sourcePhaseStale, out[0].Phase)
}

// The binding reports the worst any reader saw, so a third phase joins the
// existing order rather than letting the last reader to reconcile decide.
func TestSourceStatusPhasePrecedence(t *testing.T) {
	resolved := sources.SourceResolutionStatus{Name: "registry", Phase: sourcePhaseResolved,
		ConsumedFields: map[string]interface{}{"data.image": "nginx"}}
	stale := sources.SourceResolutionStatus{Name: "registry", Phase: sourcePhaseResolved, Stale: true,
		ConsumedFields: map[string]interface{}{"data.image": "nginx"}}
	failed := sources.SourceResolutionStatus{Name: "registry", Phase: sourcePhaseFailed, Message: "gone",
		ConsumedFields: map[string]interface{}{"data.image": "nginx"}}

	for _, tc := range []struct {
		name  string
		first sources.SourceResolutionStatus
		then  sources.SourceResolutionStatus
		want  string
	}{
		{"stale beats resolved", resolved, stale, sourcePhaseStale},
		{"stale beats resolved whichever order", stale, resolved, sourcePhaseStale},
		{"failed beats stale", stale, failed, sourcePhaseFailed},
		{"failed beats stale whichever order", failed, stale, sourcePhaseFailed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := handlerFor(nil, v1beta1.ApplicationSource{Name: "registry", Type: "configmap"})
			h.recordSourceResolution(sourceKindComponent, "web", "webservice", "local", "default",
				map[string]sources.SourceResolutionStatus{"registry": tc.first})
			h.recordSourceResolution(sourceKindComponent, "api", "webservice", "remote", "default",
				map[string]sources.SourceResolutionStatus{"registry": tc.then})
			require.Equal(t, tc.want, h.sourceStatusList()[0].Phase)
		})
	}
}

// A binding whose resolution failed reports Failed, even when the render that
// failed never produced a component status.
//
// Unused is the value a binding starts at so that "nothing read it" stays
// distinguishable from "it failed", which is what the comment on
// sourceStatusList says it is for. A render that aborts used to skip the
// recording entirely and leave the binding at that starting value, so the one
// case the default exists to be distinguishable FROM reported as the default.
func TestSourceStatusListReportsAFailureFromAnAbortedRender(t *testing.T) {
	r := require.New(t)
	h := handlerFor(nil, v1beta1.ApplicationSource{Name: "registry", Type: "configmap"})
	h.Client = unreachableClient{}
	h.currentAppRev = &v1beta1.ApplicationRevision{
		Spec: v1beta1.ApplicationRevisionSpec{ApplicationRevisionCompressibleFields: v1beta1.ApplicationRevisionCompressibleFields{
			Application: v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "default"}},
		}},
	}

	// The resolver records what it knows before handing the error back, so the
	// failure is in the render context when the render gives up.
	pCtx := velaprocess.NewContext(velaprocess.ContextData{AppName: "app", CompName: "web", Namespace: "default"})
	pCtx.PushData(sources.SourceResolutionStatusKey, map[string]sources.SourceResolutionStatus{
		"registry": {Name: "registry", Type: "configmap", Phase: sourcePhaseFailed,
			Message: "read configmap platform-settings: not found"},
	})
	comp := &appfile.Component{
		Name:               "web",
		Type:               "webservice",
		CapabilityCategory: types.TerraformCategory,
		FullTemplate:       &appfile.Template{Reference: common.WorkloadTypeDescriptor{}},
		Ctx:                pCtx,
	}

	_, _, _, _, err := h.collectHealthStatus(context.Background(), comp, "", false)
	r.Error(err, "the render aborts")

	out := h.sourceStatusList()
	r.Len(out, 1)
	r.Equal(sourcePhaseFailed, out[0].Phase, "a failed binding must not report as unread")
}

// unreachableClient fails every read, which is what makes the render give up
// before it can produce a component status.
type unreachableClient struct{ client.Client }

func (unreachableClient) Get(_ context.Context, _ client.ObjectKey, _ client.Object, _ ...client.GetOption) error {
	return errors.New("the cluster is unreachable")
}

// The binding's phase and the entry it came from must agree. They are written
// in two places from the same resolution, so naming the phase in one and not
// the other would have a binding report Stale while the resolution it is
// reporting says Resolved.
func TestStaleIsReportedOnTheResolutionToo(t *testing.T) {
	r := require.New(t)
	stale := sources.SourceResolutionStatus{
		Name: "registry", Phase: sourcePhaseResolved, Stale: true,
		Config: "configmap-local-abc", ExpiresAt: "2026-10-18T00:00:00Z",
		Message:        "refresh failed; serving stale cached value",
		ConsumedFields: map[string]interface{}{"data.image": "nginx"},
	}

	// A resolution recorded for the first time.
	h := handlerFor(nil, v1beta1.ApplicationSource{Name: "registry", Type: "configmap"})
	h.recordSourceResolution(sourceKindComponent, "web", "webservice", "local", "default",
		map[string]sources.SourceResolutionStatus{"registry": stale})
	out := h.sourceStatusList()
	r.Len(out[0].Resolutions, 1)
	r.Equal(sourcePhaseStale, out[0].Phase)
	r.Equal(sourcePhaseStale, out[0].Resolutions[0].Phase, "the entry agrees with the binding")

	// And one updated, where a second reader found the same entry stale.
	h = handlerFor(nil, v1beta1.ApplicationSource{Name: "registry", Type: "configmap"})
	fresh := stale
	fresh.Stale = false
	fresh.Message = ""
	h.recordSourceResolution(sourceKindComponent, "web", "webservice", "local", "default",
		map[string]sources.SourceResolutionStatus{"registry": fresh})
	h.recordSourceResolution(sourceKindComponent, "api", "webservice", "remote", "default",
		map[string]sources.SourceResolutionStatus{"registry": stale})
	out = h.sourceStatusList()
	r.Len(out[0].Resolutions, 1, "one storage key, one entry")
	r.Equal(sourcePhaseStale, out[0].Resolutions[0].Phase)
	r.Contains(out[0].Resolutions[0].Message, "serving stale")
}

// An Application deployed against an upstream that is already broken fails in
// the workflow's own render, before any component status exists. The report in
// collectHealthStatus cannot reach that, so the render paths report what they
// resolved themselves.
func TestRenderFailureReportsWhatItResolved(t *testing.T) {
	r := require.New(t)
	h := handlerFor(nil, v1beta1.ApplicationSource{Name: "registry", Type: "configmap"})

	pCtx := velaprocess.NewContext(velaprocess.ContextData{AppName: "app", CompName: "web", Namespace: "default"})
	pCtx.PushData(sources.SourceResolutionStatusKey, map[string]sources.SourceResolutionStatus{
		"registry": {Name: "registry", Type: "configmap", Phase: sourcePhaseFailed,
			Message: "read configmap platform-settings: not found"},
	})

	h.recordRenderFailureSourceReads(&appfile.Component{Name: "web", Type: "webservice", Ctx: pCtx}, "", "")

	out := h.sourceStatusList()
	r.Len(out, 1)
	r.Equal(sourcePhaseFailed, out[0].Phase, "a first deploy against a broken upstream says so")
	// Nothing was stored, so the entry carries no storage key, but it does carry
	// the reason, which is the whole point of reporting from here.
	r.Len(out[0].Resolutions, 1)
	r.Empty(out[0].Resolutions[0].StorageKey)
	r.Contains(out[0].Resolutions[0].Message, "platform-settings")

	// Nothing to report from is not a failure to report.
	empty := handlerFor(nil, v1beta1.ApplicationSource{Name: "registry", Type: "configmap"})
	empty.recordRenderFailureSourceReads(nil, "", "")
	r.Equal(sourcePhaseUnused, empty.sourceStatusList()[0].Phase,
		"a render that failed before resolving anything leaves the binding unread")
}

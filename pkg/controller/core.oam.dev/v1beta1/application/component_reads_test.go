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
	"fmt"
	"strings"
	"sync"
	"testing"

	wfTypesv1alpha1 "github.com/kubevela/pkg/apis/oam/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/appfile"
	"github.com/oam-dev/kubevela/pkg/definition/propexpr"
	"github.com/oam-dev/kubevela/pkg/features"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/resourcekeeper"
	"github.com/oam-dev/kubevela/pkg/sources"
	common2 "github.com/oam-dev/kubevela/pkg/utils/common"
)

// fakeCluster is a set of producers by placement: "db@east/orders" -> its
// workload, healthy or not, as status and the resource tracker report them.
type fakeCluster struct {
	workloads map[string]map[string]interface{}
	unhealthy map[string]bool
	mu        sync.Mutex
	calls     []string
}

func (f *fakeCluster) objects(_ context.Context, component, cluster, ns string, _ placementCounts) ([]*unstructured.Unstructured, []*unstructured.Unstructured, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, component+"@"+normaliseCluster(cluster)+"/"+ns)
	for key, w := range f.workloads {
		name, placement, _ := strings.Cut(key, "@")
		c, n, _ := strings.Cut(placement, "/")
		if name == component && normaliseCluster(c) == normaliseCluster(cluster) && n == ns {
			return []*unstructured.Unstructured{{Object: w}}, nil, nil
		}
	}
	return nil, nil, nil
}

func (f *fakeCluster) placements() []common.ApplicationComponentStatus {
	var out []common.ApplicationComponentStatus
	for key := range f.workloads {
		name, placement, _ := strings.Cut(key, "@")
		cluster, ns, _ := strings.Cut(placement, "/")
		out = append(out, common.ApplicationComponentStatus{Name: name, Cluster: cluster, Namespace: ns, Healthy: !f.unhealthy[key]})
	}
	return out
}

func endpoint(host string) map[string]interface{} {
	return map[string]interface{}{"status": map[string]interface{}{"endpoint": host}}
}

func reader(expr string) common.ApplicationComponent {
	raw, _ := json.Marshal(map[string]interface{}{"image": "api", "url": expr})
	return common.ApplicationComponent{Name: "api", Type: "webservice", Properties: &runtime.RawExtension{Raw: raw}}
}

func delivered(t *testing.T, scope map[string]interface{}) map[string]interface{} {
	t.Helper()
	require.NotNil(t, scope)
	return scope
}

func deliveryFor(f *fakeCluster) *componentReadDelivery {
	return newComponentReadDelivery(true, "default", f.placements, f.objects)
}

var db = common.ApplicationComponent{Name: "db", Type: "postgres"}

func TestDeliverBesideTheReader(t *testing.T) {
	f := &fakeCluster{workloads: map[string]map[string]interface{}{
		"db@east/orders": endpoint("db.east"),
		"db@data/orders": endpoint("db.data"),
	}}
	d := deliveryFor(f)
	api := reader(`$(component.db.output.status.endpoint)`)

	got, err := d.deliver(context.Background(), api, []common.ApplicationComponent{db, api}, "east", "orders")
	require.NoError(t, err)
	require.Equal(t, endpoint("db.east"), delivered(t, got)["db"].(map[string]interface{})["output"])

	// One check per placement per reconcile, however many renders ask.
	_, err = d.deliver(context.Background(), api, []common.ApplicationComponent{db, api}, "east", "orders")
	require.NoError(t, err)
	require.Equal(t, []string{"db@east/orders"}, f.calls)

	t.Run("a component that reads nothing is untouched", func(t *testing.T) {
		got, err := d.deliver(context.Background(), db, []common.ApplicationComponent{db, api}, "east", "orders")
		require.NoError(t, err)
		require.Nil(t, got)
	})

	t.Run("an unhealthy producer means wait", func(t *testing.T) {
		f.unhealthy = map[string]bool{"db@east/orders": true}
		defer func() { f.unhealthy = nil }()
		_, err := deliveryFor(f).deliver(context.Background(), api, []common.ApplicationComponent{db, api}, "east", "orders")
		require.True(t, sources.IsComponentReadNotReady(err), "%v", err)
		require.ErrorContains(t, err, `waiting for component "db" in east/orders to be healthy`)
	})

	t.Run("a producer placed only elsewhere names the fix", func(t *testing.T) {
		_, err := deliveryFor(f).deliver(context.Background(), api, []common.ApplicationComponent{db, api}, "west", "orders")
		require.True(t, sources.IsComponentReadNotReady(err), "%v", err)
		require.ErrorContains(t, err, `component.db.cluster("<cluster>")`)
	})

	t.Run("with one placement the fix is spelled out", func(t *testing.T) {
		one := &fakeCluster{workloads: map[string]map[string]interface{}{"db@data/orders": endpoint("db.data")}}
		_, err := deliveryFor(one).deliver(context.Background(), api, []common.ApplicationComponent{db, api}, "east", "orders")
		require.ErrorContains(t, err, `read it there with component.db.cluster("data")`)
	})

	t.Run("the hint is the shortest read that reaches it", func(t *testing.T) {
		for placedAt, want := range map[string]string{
			"db@east/team-a": `component.db.namespace("team-a")`,
			"db@data/team-a": `component.db.cluster("data").namespace("team-a")`,
		} {
			one := &fakeCluster{workloads: map[string]map[string]interface{}{placedAt: endpoint("x")}}
			_, err := deliveryFor(one).deliver(context.Background(), api, []common.ApplicationComponent{db, api}, "east", "orders")
			require.ErrorContains(t, err, "read it there with "+want)
		}
	})
}

// along follows placement calls through a delivered component, as the CEL
// functions do.
func along(t *testing.T, c map[string]interface{}, calls ...string) interface{} {
	t.Helper()
	node := delivered(t, c)["db"]
	for _, call := range calls {
		node = node.(map[string]interface{})[propexpr.QualifiedKey].(map[string]interface{})[call]
	}
	return node
}

func TestDeliverAtANamedPlacement(t *testing.T) {
	f := &fakeCluster{workloads: map[string]map[string]interface{}{
		"db@data/orders":  endpoint("db.data"),
		"db@multi/a":      endpoint("db.multi.a"),
		"db@multi/b":      endpoint("db.multi.b"),
		"db@east/orders2": endpoint("db.east.orders2"),
	}}
	cluster := func(c string) string { return propexpr.PlacementCall(propexpr.PlaceCluster, c) }
	namespace := func(n string) string { return propexpr.PlacementCall(propexpr.PlaceNamespace, n) }

	for _, tc := range []struct {
		expr     string
		calls    []string
		want     string
		err      string
		notReady bool
	}{
		{expr: `$(component.db.cluster("data").output.status.endpoint)`, calls: []string{cluster("data")}, want: "db.data"},
		{expr: `$(component.db.cluster("multi").namespace("b").output.status.endpoint)`,
			calls: []string{cluster("multi"), namespace("b")}, want: "db.multi.b"},
		// namespace() alone stays in the reader's cluster, east.
		{expr: `$(component.db.namespace("orders2").output.status.endpoint)`, calls: []string{namespace("orders2")}, want: "db.east.orders2"},
		{expr: `$(component.db.cluster("multi").output.status.endpoint)`, err: `is in 2 namespaces of cluster "multi"`},
		{expr: `$(component.db.cluster("nowhere").output.status.endpoint)`, err: `has not been placed in cluster "nowhere" yet`, notReady: true},
	} {
		t.Run(tc.expr, func(t *testing.T) {
			api := reader(tc.expr)
			got, err := deliveryFor(f).deliver(context.Background(), api, []common.ApplicationComponent{db, api}, "east", "orders")
			if tc.err != "" {
				require.ErrorContains(t, err, tc.err)
				require.Equal(t, tc.notReady, sources.IsComponentReadNotReady(err))
				return
			}
			require.NoError(t, err)
			require.Equal(t, endpoint(tc.want), along(t, got, tc.calls...).(map[string]interface{})["output"])
		})
	}
}

// A trait's reads are answered on its component, where its render finds them.
func TestDeliverATraitsReads(t *testing.T) {
	f := &fakeCluster{workloads: map[string]map[string]interface{}{"db@east/orders": endpoint("db.east")}}
	api := reader("plain")
	api.Traits = []common.ApplicationTrait{{Type: "labels", Properties: &runtime.RawExtension{
		Raw: []byte(`{"db":"$(component.db.output.status.endpoint)"}`)}}}
	got, err := deliveryFor(f).deliver(context.Background(), api, []common.ApplicationComponent{db, api}, "east", "orders")
	require.NoError(t, err)
	require.Contains(t, delivered(t, got), "db")
}

// A producer whose step finished in an earlier reconcile is only in the stored
// status: this reconcile's services start empty and hold what it touched.
func TestPlacementsIncludeEarlierReconciles(t *testing.T) {
	h := &AppHandler{app: &v1beta1.Application{Status: common.AppStatus{Services: []common.ApplicationComponentStatus{
		{Name: "db", Cluster: "local", Namespace: "orders"},
	}}}}
	h.services = []common.ApplicationComponentStatus{{Name: "api", Cluster: "east", Namespace: "orders"}}
	got := h.componentReads().placements()
	require.ElementsMatch(t, []common.ApplicationComponentStatus{
		{Name: "db", Cluster: "local", Namespace: "orders"},
		{Name: "api", Cluster: "east", Namespace: "orders"},
	}, got)
}

// However the author spells the local cluster, a producer placed there is
// found: a step that names no cluster records it blank, a deploy step as "local".
func TestDeliverMatchesEitherLocalSpelling(t *testing.T) {
	for _, recorded := range []string{"", "local"} {
		t.Run(fmt.Sprintf("recorded %q", recorded), func(t *testing.T) {
			f := &fakeCluster{workloads: map[string]map[string]interface{}{"db@" + recorded + "/team-a": endpoint("db.a")}}
			api := reader(`$(component.db.cluster("local").namespace("team-a").output.status.endpoint)`)
			_, err := deliveryFor(f).deliver(context.Background(), api, []common.ApplicationComponent{db, api}, "", "team-b")
			require.NoError(t, err)
		})
	}
}

// A reader that has never been applied has no service entry for status
// collection to update, so the reason it waits is recorded where it is held.
func TestWaitingReaderIsReported(t *testing.T) {
	h := &AppHandler{app: &v1beta1.Application{}}
	h.app.Namespace = "shop"
	api := common.ApplicationComponent{Name: "api", Type: "webservice"}

	h.recordWaiting(api, "east", "", sources.ComponentReadNotReady{Reason: "waiting for component \"db\""})
	require.Len(t, h.services, 1)
	got := h.services[0]
	require.Equal(t, "api", got.Name)
	require.Equal(t, "east", got.Cluster)
	require.Equal(t, "shop", got.Namespace, "an empty placement namespace is the Application's")
	require.False(t, got.Healthy)
	require.Equal(t, `waiting for component "db"`, got.Message)

	// A later reason replaces the earlier one rather than adding a second entry.
	h.recordWaiting(api, "east", "", sources.ComponentReadNotReady{Reason: "waiting again"})
	require.Len(t, h.services, 1)
	require.Equal(t, "waiting again", h.services[0].Message)
}

// Without the opt-in, $( ) is ordinary text - a shell-style variable, say - and
// a component carrying it is rendered untouched rather than parsed.
func TestDeliverLeavesAnApplicationThatHasNotOptedIn(t *testing.T) {
	f := &fakeCluster{}
	d := newComponentReadDelivery(false, "default", f.placements, f.objects)
	api := reader("$(SVC_HOST)")
	got, err := d.deliver(context.Background(), api, []common.ApplicationComponent{db, api}, "east", "orders")
	require.NoError(t, err)
	require.Nil(t, got)
	require.Empty(t, f.calls)
}

// Several readers, each reading the same placement twice, delivered at once as
// the deploy step's parallel renders do, then marshalled. A cached view written
// into by one while another reads it is a fatal concurrent map access; run with
// -race to see it.
func TestDeliverSharesNoWritableMaps(t *testing.T) {
	f := &fakeCluster{workloads: map[string]map[string]interface{}{"db@data/orders": endpoint("db.data")}}
	d := deliveryFor(f)
	r := reader(`$(component.db.cluster("data").output.status.endpoint):$(component.db.cluster("data").namespace("orders").output.status.endpoint)`)
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			scope, err := d.deliver(context.Background(), r, []common.ApplicationComponent{db, r}, "east", "orders")
			assert.NoError(t, err)
			_, err = json.Marshal(scope)
			assert.NoError(t, err)
		}()
	}
	wg.Wait()
	for _, cached := range d.views {
		require.NotContains(t, cached, propexpr.QualifiedKey, "a cached view must never be written into")
	}
}

// componentReadsKeeper answers only what componentObjects asks of a resource
// keeper.
type componentReadsKeeper struct {
	resourcekeeper.ResourceKeeper
	managed []v1beta1.ManagedResource
}

func (k componentReadsKeeper) ComponentResources(component string) []v1beta1.ManagedResource {
	var out []v1beta1.ManagedResource
	for _, mr := range k.managed {
		if mr.Component == component {
			out = append(out, mr)
		}
	}
	return out
}

func trackedConfigMap(component, cluster, ns, name string) v1beta1.ManagedResource {
	return v1beta1.ManagedResource{
		ClusterObjectReference: common.ClusterObjectReference{Cluster: cluster,
			ObjectReference: corev1.ObjectReference{APIVersion: "v1", Kind: "ConfigMap", Namespace: ns, Name: name}},
		OAMObjectReference: common.OAMObjectReference{Component: component},
	}
}

func configMap(ns, name string, labels map[string]string) *corev1.ConfigMap {
	return &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: name, Labels: labels}}
}

// A producer's view is what was applied for it at that placement, live: the
// workload and its labelled trait resources, from the tracker's record rather
// than a fresh render, so overrides and replica keys it was applied with hold.
func TestComponentObjects(t *testing.T) {
	cli := fake.NewClientBuilder().WithScheme(common2.Scheme).WithObjects(
		configMap("orders", "db", nil),
		configMap("orders", "db-svc", map[string]string{oam.TraitResource: "svc"}),
		configMap("other", "db", nil),
		configMap("orders", "rep-a", map[string]string{oam.LabelReplicaKey: "a"}),
		configMap("orders", "rep-b", map[string]string{oam.LabelReplicaKey: "b"}),
	).Build()
	h := &AppHandler{Client: cli, resourceKeeper: componentReadsKeeper{managed: []v1beta1.ManagedResource{
		// The local cluster recorded blank, as a step naming no cluster does.
		trackedConfigMap("db", "", "orders", "db"),
		trackedConfigMap("db", "", "orders", "db-svc"),
		trackedConfigMap("db", "", "other", "db"),
		trackedConfigMap("db", "east", "orders", "db"),
		trackedConfigMap("rep", "", "orders", "rep-a"),
		trackedConfigMap("rep", "", "orders", "rep-b"),
	}}}

	workloads, outputs, err := h.componentObjects(context.Background(), "db", "local", "orders", placementCounts{inCluster: 2, total: 3}, nil)
	require.NoError(t, err)
	require.Len(t, workloads, 1)
	require.Equal(t, "db", workloads[0].GetName())
	require.Equal(t, "orders", workloads[0].GetNamespace())
	require.Len(t, outputs, 1)
	require.Equal(t, "db-svc", outputs[0].GetName())

	// Every replica's workload is returned; the reader's replica picks one.
	workloads, _, err = h.componentObjects(context.Background(), "rep", "local", "orders", placementCounts{inCluster: 1, total: 1}, nil)
	require.NoError(t, err)
	require.Len(t, workloads, 2)
}

// Replicas of a reader are paired with the producer's replica of the same key,
// from one fetch of the producer: the replica is chosen per reader, not fetched.
func TestDeliverKeepsReplicasApart(t *testing.T) {
	placed := func() []common.ApplicationComponentStatus {
		return []common.ApplicationComponentStatus{{Name: "db", Cluster: "local", Namespace: "orders", Healthy: true}}
	}
	replica := func(key string) *unstructured.Unstructured {
		u := &unstructured.Unstructured{Object: endpoint("db-" + key)}
		u.SetName("db-" + key)
		u.SetLabels(map[string]string{oam.LabelReplicaKey: key})
		return u
	}
	fetches := 0
	objects := func(context.Context, string, string, string, placementCounts) ([]*unstructured.Unstructured, []*unstructured.Unstructured, error) {
		fetches++
		return []*unstructured.Unstructured{replica("a"), replica("b")}, nil, nil
	}
	d := newComponentReadDelivery(true, "default", placed, objects)
	api := reader(`$(component.db.output.status.endpoint)`)
	for _, key := range []string{"a", "b", "a"} {
		scope, err := d.deliver(contextWithReplicaKey(context.Background(), key), api, []common.ApplicationComponent{db, api}, "local", "orders")
		require.NoError(t, err)
		output := scope["db"].(map[string]interface{})["output"].(map[string]interface{})
		require.Equal(t, endpoint("db-" + key)["status"], output["status"], "replica %s", key)
	}
	require.Equal(t, 1, fetches)

	_, err := d.deliver(context.Background(), api, []common.ApplicationComponent{db, api}, "local", "orders")
	require.ErrorContains(t, err, "has 2 workloads there")
}

// A placement recorded blank in one list and with the Application's namespace in
// the other is one placement.
func TestPlacedAtCountsAPlacementOnce(t *testing.T) {
	d := newComponentReadDelivery(true, "shop", func() []common.ApplicationComponentStatus {
		return []common.ApplicationComponentStatus{
			{Name: "db", Cluster: "local", Namespace: "shop"}, {Name: "db", Cluster: "", Namespace: ""},
			{Name: "db", Cluster: "east", Namespace: "shop"},
		}
	}, nil)
	require.Len(t, d.placedAt("db"), 2)
}

// A trait output recorded away from the producer's placement still belongs to
// it: a control-plane-only trait's on the hub, and a trait writing into another
// namespace of the same cluster when the producer has only one placement there.
func TestComponentObjectsFindsOutputsPlacedElsewhere(t *testing.T) {
	cli := fake.NewClientBuilder().WithScheme(common2.Scheme).WithObjects(
		configMap("orders", "db", nil),
		configMap("app-ns", "db-hub", map[string]string{oam.TraitResource: "hub"}),
		configMap("infra", "db-cert", map[string]string{oam.TraitResource: "cert"}),
	).Build()
	hubOutput := trackedConfigMap("db", "", "app-ns", "db-hub")
	hubOutput.Trait = "hub-only"
	certOutput := trackedConfigMap("db", "east", "infra", "db-cert")
	certOutput.Trait = "cert"
	h := &AppHandler{Client: cli,
		app: &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Namespace: "app-ns"}},
		currentAppRev: &v1beta1.ApplicationRevision{Spec: v1beta1.ApplicationRevisionSpec{ApplicationRevisionCompressibleFields: v1beta1.ApplicationRevisionCompressibleFields{
			// Keyed by something other than the definition's name, which is what the
			// redirect matches on.
			TraitDefinitions: map[string]*v1beta1.TraitDefinition{"hub-only@v2": {
				ObjectMeta: metav1.ObjectMeta{Name: "hub-only"}, Spec: v1beta1.TraitDefinitionSpec{ControlPlaneOnly: true}}},
		}}},
		resourceKeeper: componentReadsKeeper{managed: []v1beta1.ManagedResource{
			trackedConfigMap("db", "east", "orders", "db"), hubOutput, certOutput,
		}}}

	cpOnly := controlPlaneOnlyTraits(h.currentAppRev)
	names := func(counts placementCounts) []string {
		_, outputs, err := h.componentObjects(context.Background(), "db", "east", "orders", counts, cpOnly)
		require.NoError(t, err)
		var out []string
		for _, o := range outputs {
			out = append(out, o.GetName())
		}
		return out
	}
	require.ElementsMatch(t, []string{"db-hub", "db-cert"}, names(placementCounts{inCluster: 1, total: 1}))

	// A second placement in the cluster: an output in another namespace, or on
	// the hub, could be either's, so both are left out.
	require.Empty(t, names(placementCounts{inCluster: 2, total: 2}))

	// A second placement anywhere: every placement's control-plane-only output
	// lands on the hub under one name, so it could be either's.
	require.ElementsMatch(t, []string{"db-cert"}, names(placementCounts{inCluster: 1, total: 2}))
}

// A render delivers the reads of the traits it renders and no others: a
// post-dispatch trait left out of the workflow's render is not waited on.
func TestWithRenderedTraits(t *testing.T) {
	comp := common.ApplicationComponent{Name: "api", Traits: []common.ApplicationTrait{{Type: "a"}, {Type: "late"}, {Type: "b"}}}
	all := []*appfile.Trait{{Name: "a"}, {Name: "late"}, {Name: "b"}}
	got := withRenderedTraits(comp, all, []*appfile.Trait{all[0], all[2]})
	require.Equal(t, []common.ApplicationTrait{{Type: "a"}, {Type: "b"}}, got.Traits)
	require.Len(t, comp.Traits, 3, "the component itself is not changed")

	// Should parsing ever not line up one trait per entry, every trait is kept.
	require.Equal(t, comp, withRenderedTraits(comp, all[:2], all[:1]))
}

// A cluster that is not registered - a typo that got past admission, or one
// since detached - is an error in the reader's status, not a wait for ever.
func TestDeliverRefusesAnUnregisteredCluster(t *testing.T) {
	f := &fakeCluster{workloads: map[string]map[string]interface{}{"db@east/orders": endpoint("db.east")}}
	d := deliveryFor(f)
	d.clusterRegistered = func(_ context.Context, cluster string) bool { return cluster == "east" }

	api := reader(`$(component.db.cluster("eats").output.status.endpoint)`)
	_, err := d.deliver(context.Background(), api, []common.ApplicationComponent{db, api}, "local", "orders")
	require.ErrorContains(t, err, `no cluster "eats" is registered`)
	require.False(t, sources.IsComponentReadNotReady(err))

	api = reader(`$(component.db.cluster("east").output.status.endpoint)`)
	_, err = d.deliver(context.Background(), api, []common.ApplicationComponent{db, api}, "local", "orders")
	require.NoError(t, err)
}

// A reader already applied keeps what its status records; only its health and
// the reason change.
func TestWaitingReaderKeepsItsStatus(t *testing.T) {
	h := &AppHandler{app: &v1beta1.Application{}}
	h.app.Namespace = "shop"
	h.services = []common.ApplicationComponentStatus{{Name: "api", Cluster: "east", Namespace: "shop", Healthy: true,
		WorkloadDefinition: common.WorkloadGVK{APIVersion: "apps/v1", Kind: "Deployment"}}}
	h.recordWaiting(common.ApplicationComponent{Name: "api"}, "east", "", sources.ComponentReadNotReady{Reason: "waiting"})
	require.Len(t, h.services, 1)
	require.False(t, h.services[0].Healthy)
	require.Equal(t, "waiting", h.services[0].Message)
	require.Equal(t, "Deployment", h.services[0].WorkloadDefinition.Kind)
	require.True(t, h.readsWaiting)
}

// The engine orders a reader's step after its producer's through dependsOn,
// added to the instance's steps and never to the Application's own.
func TestWorkflowInstanceOrdersReadersAfterProducers(t *testing.T) {
	before := map[string]bool{
		string(features.EnableCelExpressions):      utilfeature.DefaultMutableFeatureGate.Enabled(features.EnableCelExpressions),
		string(features.RequireCelExpressionOptIn): utilfeature.DefaultMutableFeatureGate.Enabled(features.RequireCelExpressionOptIn),
	}
	require.NoError(t, utilfeature.DefaultMutableFeatureGate.SetFromMap(map[string]bool{
		string(features.EnableCelExpressions): true, string(features.RequireCelExpressionOptIn): true}))
	t.Cleanup(func() { require.NoError(t, utilfeature.DefaultMutableFeatureGate.SetFromMap(before)) })

	applyStep := func(c string) wfTypesv1alpha1.WorkflowStep {
		return wfTypesv1alpha1.WorkflowStep{WorkflowStepBase: wfTypesv1alpha1.WorkflowStepBase{
			Name: c, Type: "apply-component", Properties: &runtime.RawExtension{Raw: []byte(`{"component":"` + c + `"}`)}}}
	}
	api := reader("$(component.db.output.status.endpoint)")
	app := &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: "shop", Namespace: "default",
		Annotations: map[string]string{oam.AnnotationCelExpressions: "true"}}}
	af := &appfile.Appfile{Name: "shop", Namespace: "default", AppAnnotations: app.Annotations,
		Components:    []common.ApplicationComponent{db, api},
		WorkflowSteps: []wfTypesv1alpha1.WorkflowStep{applyStep("db"), applyStep("api")},
		WorkflowMode:  &wfTypesv1alpha1.WorkflowExecuteMode{Steps: "DAG", SubSteps: "DAG"}}

	af.Dependencies = sources.Dependencies(af.Components, af.AppAnnotations)
	instance, err := generateWorkflowInstance(af, app, af.StepsWithReadDependencies(af.WorkflowSteps), nil)
	require.NoError(t, err)
	require.Equal(t, []string{"db"}, instance.Steps[1].DependsOn)
	require.Empty(t, af.WorkflowSteps[1].DependsOn, "the Appfile's steps are left as parsed")
}

// A render through ComponentRender records where its component is placed for the
// reads alone: the template's context keeps the Application's namespace and no
// replica key, as it always has.
func TestReadPlacementLeavesTheTemplateContext(t *testing.T) {
	ctx := contextWithReadPlacement(context.Background(), "orders", "east")
	require.Empty(t, componentNamespaceFromContext(ctx), "the template's namespace is not overridden")
	require.Empty(t, replicaKeyFromContext(ctx), "nor its replica key")
	ns, replica := readPlacementFrom(ctx)
	require.Equal(t, "orders", ns)
	require.Equal(t, "east", replica)
}

// Apply and health checks set the component's own namespace and replica key; the
// reads use those when nothing more specific was recorded.
func TestReadPlacementFallsBackToTheComponents(t *testing.T) {
	ctx := contextWithReplicaKey(contextWithComponentNamespace(context.Background(), "team-a"), "west")
	ns, replica := readPlacementFrom(ctx)
	require.Equal(t, "team-a", ns)
	require.Equal(t, "west", replica)
}

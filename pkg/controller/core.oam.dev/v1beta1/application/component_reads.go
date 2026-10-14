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
	"fmt"
	"strings"
	"sync"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	celengine "github.com/kubevela/pkg/cel"
	"golang.org/x/sync/singleflight"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	k8stypes "k8s.io/apimachinery/pkg/types"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/appfile"
	velaprocess "github.com/oam-dev/kubevela/pkg/cue/process"
	velamulticluster "github.com/oam-dev/kubevela/pkg/multicluster"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/sources"
	deployprovider "github.com/oam-dev/kubevela/pkg/workflow/providers/multicluster"
)

// componentReadDelivery answers a reader's component reads when it is rendered:
// every render, the workflow's and status collection's alike, so a read means
// the same thing wherever it is evaluated.
//
// A producer is read where the read names it, and only once status reports it
// healthy there. Its view is built from what was actually applied, as the
// resource tracker records it, fetched live: the workload under `output` and
// each trait resource under `outputs.<name>`, shaped by the deploy step's own
// OutputView. Anything not ready is a sources.ComponentReadNotReady, which leaves
// the reader unhealthy and waiting rather than failed.
type componentReadDelivery struct {
	// enabled is whether the Application opted into expressions; without it,
	// $( ) is ordinary text and there is nothing to deliver.
	enabled bool
	// objects fetches the live objects a component has applied at one placement:
	// every workload (one per replica) and every trait output. counts says how
	// many placements the component has, which decides whether an output recorded
	// away from this one can only be this one's.
	objects func(ctx context.Context, component, cluster, ns string, counts placementCounts) ([]*unstructured.Unstructured, []*unstructured.Unstructured, error)
	// placements is where each component has been placed and whether it is
	// healthy there, as status reports it.
	placements func() []common.ApplicationComponentStatus
	// appNamespace is what an empty placement namespace means.
	appNamespace string
	// clusterRegistered reports whether a cluster a read names is registered.
	// Nil takes every cluster as registered.
	clusterRegistered func(ctx context.Context, cluster string) bool

	mu sync.Mutex
	// fetched holds each producer's objects by placement, and views each view
	// built from them by the workload chosen, for one reconcile, since every
	// render of every reader in it would otherwise fetch again. A view is shared,
	// so never written after it is stored: a result takes a copy.
	fetched  map[string]fetchedObjects
	inflight singleflight.Group
	views    map[string]map[string]interface{}
	// reads holds each component's parsed reads, keyed by its properties, for the
	// same reason.
	reads map[string][]sources.ComponentRead
	cc    *cue.Context
}

func newComponentReadDelivery(enabled bool, appNamespace string,
	placements func() []common.ApplicationComponentStatus,
	objects func(context.Context, string, string, string, placementCounts) ([]*unstructured.Unstructured, []*unstructured.Unstructured, error)) *componentReadDelivery {
	return &componentReadDelivery{
		enabled:      enabled,
		appNamespace: appNamespace, placements: placements, objects: objects,
		fetched: map[string]fetchedObjects{}, views: map[string]map[string]interface{}{},
		reads: map[string][]sources.ComponentRead{}, cc: cuecontext.New(),
	}
}

// deliver answers comp's reads, returning the `component` scope its render
// evaluates against, or nil when it reads nothing. all is every component in
// the Application; cluster and ns are where this render places comp.
//
// The scope travels with the render's context and never with comp: properties
// feed the component's revision hash, and a producer's live object changes on
// every status update.
func (d *componentReadDelivery) deliver(ctx context.Context, comp common.ApplicationComponent,
	all []common.ApplicationComponent, cluster, ns string) (map[string]interface{}, error) {
	if !d.enabled {
		return nil, nil
	}
	reads, err := d.readsOf(comp)
	if err != nil || len(reads) == 0 {
		return nil, err
	}
	byName := map[string]common.ApplicationComponent{}
	for _, c := range all {
		byName[c.Name] = c
	}

	scope := map[string]interface{}{}
	for _, r := range reads {
		producer, ok := byName[r.Producer]
		if !ok {
			return nil, fmt.Errorf("component %q reads %s, but the application has no component %q", comp.Name, r, r.Producer)
		}
		entry, _ := scope[r.Producer].(map[string]interface{})
		if entry == nil {
			entry = map[string]interface{}{}
			scope[r.Producer] = entry
		}
		target, err := r.Target()
		if err != nil {
			return nil, err
		}
		v, err := d.resolve(ctx, comp.Name, producer, target, cluster, ns)
		if err != nil {
			return nil, err
		}
		placeAlong(entry, r.Placement, v)
	}
	return scope, nil
}

// readsOf is comp's reads, parsed once per reconcile. A component with no $( )
// anywhere cannot read one, and is answered without parsing.
func (d *componentReadDelivery) readsOf(comp common.ApplicationComponent) ([]sources.ComponentRead, error) {
	var key strings.Builder
	key.WriteString(comp.Name)
	if comp.Properties != nil {
		key.Write(comp.Properties.Raw)
	}
	for _, tr := range comp.Traits {
		key.WriteByte(0)
		if tr.Properties != nil {
			key.Write(tr.Properties.Raw)
		}
	}
	if !strings.Contains(key.String(), "$(") {
		return nil, nil
	}
	d.mu.Lock()
	cached, ok := d.reads[key.String()]
	d.mu.Unlock()
	if ok {
		return cached, nil
	}
	reads, err := sources.ComponentReads(comp)
	if err != nil {
		return nil, err
	}
	d.mu.Lock()
	d.reads[key.String()] = reads
	d.mu.Unlock()
	return reads, nil
}

// resolve answers one read target for a reader placed at cluster/ns.
func (d *componentReadDelivery) resolve(ctx context.Context, reader string, producer common.ApplicationComponent,
	t sources.ReadTarget, cluster, ns string) (interface{}, error) {
	// A cluster that is not registered will never hold the producer, so waiting
	// for it would be for ever.
	if t.Cluster != "" && normaliseCluster(t.Cluster) != velamulticluster.ClusterLocalName &&
		d.clusterRegistered != nil && !d.clusterRegistered(ctx, t.Cluster) {
		return nil, fmt.Errorf("component %q reads %q in cluster %q, but no cluster %q is registered",
			reader, producer.Name, t.Cluster, t.Cluster)
	}
	switch {
	case t.Cluster == "" && t.Namespace == "":
		return d.besideReader(ctx, reader, producer, cluster, ns)
	case t.Cluster == "":
		// namespace() alone stays in the reader's cluster.
		return d.view(ctx, producer, cluster, t.Namespace)
	case t.Namespace == "":
		pns, err := d.namespaceIn(producer.Name, t.Cluster)
		if err != nil {
			return nil, err
		}
		return d.view(ctx, producer, t.Cluster, pns)
	}
	return d.view(ctx, producer, t.Cluster, t.Namespace)
}

// placeAlong stores a resolved view where the placement calls will look it up:
// each call is a key in the celengine.QualifiedKey entry of the one before, and
// no calls at all is the component's own entry. Every map it writes into is its
// own; a cached view is copied in, never stored.
func placeAlong(entry map[string]interface{}, calls []string, v interface{}) {
	node := entry
	for _, call := range calls {
		qualified, _ := node[celengine.QualifiedKey].(map[string]interface{})
		if qualified == nil {
			qualified = map[string]interface{}{}
			node[celengine.QualifiedKey] = qualified
		}
		next, _ := qualified[call].(map[string]interface{})
		if next == nil {
			next = map[string]interface{}{}
			qualified[call] = next
		}
		node = next
	}
	if view, ok := v.(map[string]interface{}); ok {
		for k, val := range view {
			node[k] = val
		}
	}
}

// besideReader is the producer in the reader's own placement.
func (d *componentReadDelivery) besideReader(ctx context.Context, reader string, producer common.ApplicationComponent,
	cluster, ns string) (map[string]interface{}, error) {
	view, err := d.view(ctx, producer, cluster, ns)
	if err == nil || !sources.IsComponentReadNotReady(err) {
		return view, err
	}
	// Placed somewhere, but not here: waiting will not help, so say what will.
	if placed := d.placedAt(producer.Name); len(placed) > 0 && !d.contains(placed, cluster, ns) {
		fix := fmt.Sprintf(`component.%s.cluster("<cluster>")`, producer.Name)
		if len(placed) == 1 {
			fix = d.placementExpr(producer.Name, placed[0][0], placed[0][1], cluster, ns)
		}
		return nil, sources.ComponentReadNotReady{Reason: fmt.Sprintf(
			"component %q reads %q beside it in %s, but %q is placed only in %s; read it there with %s",
			reader, producer.Name, d.label(cluster, ns), producer.Name, strings.Join(d.labels(placed), ", "), fix)}
	}
	return nil, err
}

// placementExpr is the shortest read of producer at pCluster/pNS, from a reader
// at cluster/ns.
func (d *componentReadDelivery) placementExpr(producer, pCluster, pNS, cluster, ns string) string {
	sameCluster := normaliseCluster(pCluster) == normaliseCluster(cluster)
	sameNS := d.namespace(pNS) == d.namespace(ns)
	switch {
	case sameCluster:
		return fmt.Sprintf("component.%s.namespace(%q)", producer, d.namespace(pNS))
	case sameNS:
		return fmt.Sprintf("component.%s.cluster(%q)", producer, normaliseCluster(pCluster))
	}
	return fmt.Sprintf("component.%s.cluster(%q).namespace(%q)", producer, normaliseCluster(pCluster), d.namespace(pNS))
}

// namespaceIn is the namespace a producer occupies in a cluster, for an at read
// that names only the cluster.
func (d *componentReadDelivery) namespaceIn(producer, cluster string) (string, error) {
	var found []string
	for _, p := range d.placedAt(producer) {
		if normaliseCluster(p[0]) == normaliseCluster(cluster) {
			found = append(found, p[1])
		}
	}
	switch len(found) {
	case 0:
		return "", sources.ComponentReadNotReady{Reason: fmt.Sprintf("component %q has not been placed in cluster %q yet", producer, cluster)}
	case 1:
		return found[0], nil
	}
	return "", fmt.Errorf("component %q is in %d namespaces of cluster %q; name one with component.%s.cluster(%q).namespace(\"<namespace>\")",
		producer, len(found), cluster, producer, cluster)
}

// placementCounts is how many placements a component has: in one cluster, and
// in all.
type placementCounts struct{ inCluster, total int }

// fetchedObjects is what a producer applied at one placement.
type fetchedObjects struct{ workloads, outputs []*unstructured.Unstructured }

// view is a healthy producer's output view at one placement, for the reader
// asking: a replicated producer's workload is the one sharing the reader's
// replica key. The map returned is shared and must not be written.
func (d *componentReadDelivery) view(ctx context.Context, producer common.ApplicationComponent, cluster, ns string) (map[string]interface{}, error) {
	svc, placed := d.placedHere(producer.Name, cluster, ns)
	switch {
	case !placed:
		return nil, sources.ComponentReadNotReady{Reason: fmt.Sprintf("waiting for component %q to be placed in %s",
			producer.Name, d.label(cluster, ns))}
	case !svc.Healthy:
		return nil, sources.ComponentReadNotReady{Reason: fmt.Sprintf("waiting for component %q in %s to be healthy",
			producer.Name, d.label(cluster, ns))}
	}
	objs, err := d.fetch(ctx, producer.Name, cluster, ns)
	if err != nil {
		return nil, err
	}
	_, replicaKey := readPlacementFrom(ctx)
	workload, err := chooseWorkload(producer.Name, objs.workloads, replicaKey)
	if err != nil {
		return nil, err
	}
	key := strings.Join([]string{producer.Name, normaliseCluster(cluster), d.namespace(ns)}, "/")
	if workload != nil {
		key += "/" + workload.GetName()
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if cached, ok := d.views[key]; ok {
		return cached, nil
	}
	cv, err := deployprovider.OutputView(workload, objs.outputs, func(s string) cue.Value { return d.cc.CompileString(s) })
	if err != nil {
		return nil, err
	}
	view := map[string]interface{}{}
	if err := cv.Decode(&view); err != nil {
		return nil, err
	}
	d.views[key] = view
	return view, nil
}

// fetch is a producer's applied objects at one placement, fetched once per
// reconcile.
func (d *componentReadDelivery) fetch(ctx context.Context, producer, cluster, ns string) (fetchedObjects, error) {
	key := strings.Join([]string{producer, normaliseCluster(cluster), d.namespace(ns)}, "/")
	d.mu.Lock()
	cached, ok := d.fetched[key]
	d.mu.Unlock()
	if ok {
		return cached, nil
	}
	// Parallel renders asking for the same placement share one fetch.
	v, err, _ := d.inflight.Do(key, func() (interface{}, error) { return d.fetchOnce(ctx, key, producer, cluster, ns) })
	if err != nil {
		return fetchedObjects{}, err
	}
	return v.(fetchedObjects), nil
}

func (d *componentReadDelivery) fetchOnce(ctx context.Context, key, producer, cluster, ns string) (fetchedObjects, error) {
	var counts placementCounts
	for _, p := range d.placedAt(producer) {
		counts.total++
		if normaliseCluster(p[0]) == normaliseCluster(cluster) {
			counts.inCluster++
		}
	}
	workloads, outputs, err := d.objects(ctx, producer, cluster, d.namespace(ns), counts)
	if err != nil {
		return fetchedObjects{}, fmt.Errorf("reading component %q in %s: %w", producer, d.label(cluster, ns), err)
	}
	got := fetchedObjects{workloads: workloads, outputs: outputs}
	d.mu.Lock()
	d.fetched[key] = got
	d.mu.Unlock()
	return got, nil
}

// chooseWorkload is the workload a reader reads: the only one, or for a
// replicated producer the one with the reader's replica key, as the deploy step
// pairs replicas.
func chooseWorkload(producer string, workloads []*unstructured.Unstructured, replicaKey string) (*unstructured.Unstructured, error) {
	switch len(workloads) {
	case 0:
		return nil, nil
	case 1:
		return workloads[0], nil
	}
	if replicaKey != "" {
		for _, w := range workloads {
			if w.GetLabels()[oam.LabelReplicaKey] == replicaKey {
				return w, nil
			}
		}
	}
	return nil, fmt.Errorf("component %q has %d workloads there; a replicated component can only be read "+
		"by a reader with the same replica key", producer, len(workloads))
}

// placedHere is a component's status entry at one placement, if it has one.
func (d *componentReadDelivery) placedHere(name, cluster, ns string) (common.ApplicationComponentStatus, bool) {
	for _, svc := range d.placements() {
		if svc.Name == name && normaliseCluster(svc.Cluster) == normaliseCluster(cluster) && d.namespace(svc.Namespace) == d.namespace(ns) {
			return svc, true
		}
	}
	return common.ApplicationComponentStatus{}, false
}

// placedAt is every cluster and namespace a component has been placed in, as
// status recorded them.
func (d *componentReadDelivery) placedAt(name string) [][2]string {
	var out [][2]string
	seen := map[string]bool{}
	for _, svc := range d.placements() {
		// One placement may be recorded with a blank namespace in one list and
		// the Application's in the other.
		key := d.label(svc.Cluster, svc.Namespace)
		if svc.Name != name || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, [2]string{svc.Cluster, svc.Namespace})
	}
	return out
}

func (d *componentReadDelivery) contains(placed [][2]string, cluster, ns string) bool {
	for _, p := range placed {
		if normaliseCluster(p[0]) == normaliseCluster(cluster) && d.namespace(p[1]) == d.namespace(ns) {
			return true
		}
	}
	return false
}

// namespace is what a placement's namespace means: empty is the Application's.
func (d *componentReadDelivery) namespace(ns string) string {
	if ns == "" {
		return d.appNamespace
	}
	return ns
}

func (d *componentReadDelivery) label(cluster, ns string) string {
	return normaliseCluster(cluster) + "/" + d.namespace(ns)
}

func (d *componentReadDelivery) labels(placed [][2]string) []string {
	out := make([]string, 0, len(placed))
	for _, p := range placed {
		out = append(out, d.label(p[0], p[1]))
	}
	return out
}

// normaliseCluster names the local cluster the same way whichever spelling a
// status entry, a tracker entry or a render context used: a step that names no
// cluster records it blank, a placement records "local".
func normaliseCluster(c string) string {
	if c == "" {
		return velamulticluster.ClusterLocalName
	}
	return c
}

// componentReads is this reconcile's delivery, reading producers from what the
// resource tracker records and resolving policies as the deploy step does.
func (h *AppHandler) componentReads() *componentReadDelivery {
	h.readDeliveryOnce.Do(func() {
		// A component whose step finished in an earlier reconcile is placed but only
		// in the stored status; this reconcile's services hold what it has touched.
		// Taken on first use, during the workflow, by which time a restart for a
		// new revision has already cleared the stored status.
		stored := append([]common.ApplicationComponentStatus(nil), h.app.Status.Services...)
		controlPlaneOnly := controlPlaneOnlyTraits(h.currentAppRev)
		h.readDelivery = newComponentReadDelivery(sources.ExpressionsEnabledFor(h.app.GetAnnotations()), h.app.Namespace,
			func() []common.ApplicationComponentStatus {
				h.mu.Lock()
				defer h.mu.Unlock()
				// This reconcile's entries first: they carry the latest health.
				return mergePlacements(h.services, stored)
			},
			func(ctx context.Context, component, cluster, ns string, counts placementCounts) ([]*unstructured.Unstructured, []*unstructured.Unstructured, error) {
				return h.componentObjects(ctx, component, cluster, ns, counts, controlPlaneOnly)
			})
		h.readDelivery.clusterRegistered = h.clusterRegistered()
	})
	return h.readDelivery
}

// componentObjects fetches a component's applied objects at one placement, live,
// from what the resource tracker records: every workload, and the resources
// carrying oam.TraitResource, as OutputView expects them.
//
// A trait output can be recorded away from its producer's placement: a
// control-plane-only trait's is redirected to the hub, and a trait may write into
// another namespace. Each is taken as this placement's only when no other
// placement could have written it: the hub one when the component has a single
// placement, the other-namespace one when it has a single placement in this
// cluster.
func (h *AppHandler) componentObjects(ctx context.Context, component, cluster, ns string, counts placementCounts,
	controlPlaneOnly map[string]bool) ([]*unstructured.Unstructured, []*unstructured.Unstructured, error) {
	var workloads, outputs []*unstructured.Unstructured
	for _, mr := range h.resourceKeeper.ComponentResources(component) {
		sameCluster := normaliseCluster(mr.Cluster) == normaliseCluster(cluster)
		here := sameCluster && (mr.Namespace == "" || mr.Namespace == ns)
		elsewhere := mr.Trait != "" &&
			((controlPlaneOnly[mr.Trait] && normaliseCluster(mr.Cluster) == velamulticluster.ClusterLocalName && counts.total == 1) ||
				(sameCluster && counts.inCluster == 1))
		if !here && !elsewhere {
			continue
		}
		u := &unstructured.Unstructured{}
		u.SetAPIVersion(mr.APIVersion)
		u.SetKind(mr.Kind)
		err := h.Client.Get(velamulticluster.ContextWithClusterName(ctx, mr.Cluster),
			k8stypes.NamespacedName{Namespace: mr.Namespace, Name: mr.Name}, u)
		if kerrors.IsNotFound(err) {
			continue
		}
		if err != nil {
			return nil, nil, err
		}
		if u.GetLabels()[oam.TraitResource] != "" {
			outputs = append(outputs, u)
			continue
		}
		if here {
			workloads = append(workloads, u)
		}
	}
	return workloads, outputs, nil
}

// controlPlaneOnlyTraits is the trait types whose resources are redirected to
// the hub, by definition name as the redirect matches them.
func controlPlaneOnlyTraits(appRev *v1beta1.ApplicationRevision) map[string]bool {
	out := map[string]bool{}
	if appRev == nil {
		return out
	}
	for _, def := range appRev.Spec.TraitDefinitions {
		if def != nil && def.Spec.ControlPlaneOnly {
			out[def.Name] = true
		}
	}
	return out
}

// mergePlacements is every distinct component placement across the lists, the
// first list's entry winning where they overlap.
func mergePlacements(lists ...[]common.ApplicationComponentStatus) []common.ApplicationComponentStatus {
	seen := map[string]bool{}
	var out []common.ApplicationComponentStatus
	for _, list := range lists {
		for _, svc := range list {
			key := svc.Name + "/" + normaliseCluster(svc.Cluster) + "/" + svc.Namespace
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, common.ApplicationComponentStatus{Name: svc.Name, Cluster: svc.Cluster, Namespace: svc.Namespace,
				Healthy: svc.Healthy})
		}
	}
	return out
}

// recordWaiting reports a reader held back on its reads: its status entry says
// why and is unhealthy, keeping everything else it records, and one is created
// for a reader not yet applied.
func (h *AppHandler) recordWaiting(comp common.ApplicationComponent, cluster, ns string, reason error) {
	if ns == "" {
		ns = h.app.Namespace
	}
	h.mu.Lock()
	h.readsWaiting = true
	for i := range h.services {
		svc := &h.services[i]
		if svc.Name == comp.Name && normaliseCluster(svc.Cluster) == normaliseCluster(cluster) &&
			(svc.Namespace == ns || (svc.Namespace == "" && ns == h.app.Namespace)) {
			svc.Healthy = false
			svc.Message = reason.Error()
			h.mu.Unlock()
			return
		}
	}
	h.mu.Unlock()
	h.addServiceStatus(true, common.ApplicationComponentStatus{
		Name:      comp.Name,
		Cluster:   cluster,
		Namespace: ns,
		Healthy:   false,
		Message:   reason.Error(),
	})
}

// withComponentScope hands a render the reads delivered for it, on the context
// the render evaluates in.
func withComponentScope(ctxData *velaprocess.ContextData, scope map[string]interface{}) {
	if scope == nil {
		return
	}
	base := ctxData.Ctx
	if base == nil {
		base = context.Background()
	}
	ctxData.Ctx = sources.WithComponentScope(base, scope)
}

// withRenderedTraits is comp with only the traits a render keeps, so that the
// reads delivered are the reads that render evaluates: a post-dispatch trait
// left out of the workflow's render must not hold it up. all is every trait as
// parsed, one per entry of comp.Traits; should that not hold, comp is returned
// whole, which delivers more reads than needed but never fewer.
func withRenderedTraits(comp common.ApplicationComponent, all, kept []*appfile.Trait) common.ApplicationComponent {
	if len(all) != len(comp.Traits) || len(kept) == len(all) {
		return comp
	}
	keep := map[*appfile.Trait]bool{}
	for _, t := range kept {
		keep[t] = true
	}
	out := comp
	out.Traits = nil
	for i, t := range all {
		if keep[t] {
			out.Traits = append(out.Traits, comp.Traits[i])
		}
	}
	return out
}

// clusterRegistered reports whether a cluster is registered on the hub, once per
// cluster per reconcile. A lookup failing for any other reason counts as
// registered: the cluster may simply be unreachable, and an unreachable
// producer is a wait, not an error.
func (h *AppHandler) clusterRegistered() func(ctx context.Context, cluster string) bool {
	var mu sync.Mutex
	known := map[string]bool{}
	return func(ctx context.Context, cluster string) bool {
		mu.Lock()
		defer mu.Unlock()
		if ok, seen := known[cluster]; seen {
			return ok
		}
		_, err := velamulticluster.GetVirtualCluster(velamulticluster.ContextInLocalCluster(ctx), h.Client, cluster)
		// IsClusterNotExists cannot take a nil error.
		known[cluster] = err == nil || !velamulticluster.IsClusterNotExists(err)
		return known[cluster]
	}
}

type readPlacementKey struct{}

type readPlacement struct{ namespace, replicaKey string }

// contextWithReadPlacement records where a render places its component, for its
// component reads alone. A render through ComponentRender keeps the template's
// context as it is; only the reads need the placement, to find their producer.
func contextWithReadPlacement(ctx context.Context, namespace, replicaKey string) context.Context {
	return context.WithValue(ctx, readPlacementKey{}, readPlacement{namespace: namespace, replicaKey: replicaKey})
}

// readPlacementFrom is where a component's reads look for their producers: what
// a render recorded for them, or else the component's own namespace and replica
// key, as apply and health checks set them.
func readPlacementFrom(ctx context.Context) (namespace, replicaKey string) {
	if p, ok := ctx.Value(readPlacementKey{}).(readPlacement); ok {
		return p.namespace, p.replicaKey
	}
	return componentNamespaceFromContext(ctx), replicaKeyFromContext(ctx)
}

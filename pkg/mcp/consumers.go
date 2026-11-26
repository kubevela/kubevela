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
	"fmt"
	"sort"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/multicluster"
	querytypes "github.com/oam-dev/kubevela/pkg/utils/types"
)

// workloadKinds are the plain Kubernetes kinds whose pod template says what
// a consumer reads.
var workloadKinds = map[string]bool{"Deployment": true, "StatefulSet": true, "DaemonSet": true, "ReplicaSet": true, "ReplicationController": true, "Job": true, "CronJob": true}

// How a pod template reads a Secret or ConfigMap. A value read through env
// is fixed when the container starts; a mounted file updates in place.
const (
	viaEnv    = "env"
	viaVolume = "volume"
)

// consumedRef is one Secret or ConfigMap a pod template reads. An optional
// reference does not block the pod, so it is never a dependency.
type consumedRef struct {
	kind, name, key string
	via             string
	optional        bool
}

// producer is who fills a Secret or ConfigMap in this Application: a
// component that manages the object itself, or one or more exporter
// resources of a component, in one cluster and namespace. Exporters are kept
// per target key, so the key a consumer reads is judged by the exporter that
// writes it; a connection-Secret writer, which writes every key, is kept
// under the empty key.
type producer struct {
	component          string
	cluster, namespace string
	exporters          map[string]*ResourceDiagnosis
}

// unwrittenExporterFor returns an exporter of this producer whose own
// conditions say it has not written what the consumer reads: for one key,
// the exporter writing that key or the one writing the whole object; for a
// whole-object read (envFrom, a mounted Secret or ConfigMap), any exporter
// that writes into the object. Nil when none says so.
func (p producer) unwrittenExporterFor(key string) *ResourceDiagnosis {
	unwritten := func(e *ResourceDiagnosis) bool {
		return e != nil && e.Export != nil && e.Export.Written != nil && !*e.Export.Written
	}
	if key != "" {
		if e := p.exporters[key]; unwritten(e) {
			return e
		}
		if e := p.exporters[""]; unwritten(e) {
			return e
		}
		return nil
	}
	keys := make([]string, 0, len(p.exporters))
	for k := range p.exporters {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if unwritten(p.exporters[k]) {
			return p.exporters[k]
		}
	}
	return nil
}

func producerKey(cluster, namespace, kind, name string) string {
	return cluster + "\x00" + namespace + "\x00" + kind + "\x00" + name
}

// producersOf maps every Secret and ConfigMap this Application fills to the
// component that fills it. A component that manages the object directly is
// recorded first; exporters writing into it take precedence, because they
// are the more specific fact, and are kept per target key. An exporter's
// target lives in the namespace it names, else the exporter's own.
func producersOf(applied []querytypes.AppliedResource, resources []ResourceDiagnosis) map[string]producer {
	producers := map[string]producer{}
	for i := range applied {
		res := applied[i]
		if res.Kind != secretKind && res.Kind != "ConfigMap" {
			continue
		}
		producers[producerKey(clusterName(res.Cluster), res.Namespace, res.Kind, res.Name)] = producer{component: res.Component, cluster: clusterName(res.Cluster), namespace: res.Namespace}
	}
	for i := range resources {
		r := &resources[i]
		if r.Export == nil {
			continue
		}
		ns := r.Export.TargetNamespace
		if ns == "" {
			ns = r.Namespace
		}
		key := producerKey(r.Cluster, ns, r.Export.TargetKind, r.Export.TargetName)
		p, ok := producers[key]
		if !ok || p.component != r.Component || p.exporters == nil {
			p = producer{component: r.Component, cluster: r.Cluster, namespace: r.Namespace, exporters: map[string]*ResourceDiagnosis{}}
		}
		p.exporters[r.Export.TargetKey] = r
		producers[key] = p
	}
	return producers
}

// linkConsumers explains a plain workload that is not ready by what it reads
// from another component: a key an exporter says it has not written, or an
// object filled by a component whose own resources are not ready. It reads
// each such workload once, inside the diagnosis's live-read bound, and only
// when the Application fills something a workload could read. A diagnosis
// narrowed to one component holds no producers from the others, so it skips
// the read and says so. The dependsOn edge decides who owns the fix: the
// Application spec when the edge is missing, the producer's definition when
// it is declared and the producer still reported healthy too early.
func (d *deps) linkConsumers(ctx context.Context, app *v1beta1.Application, applied []querytypes.AppliedResource, out *DiagnoseApplicationOutput, reads *int, component string) {
	if component != "" {
		out.Unknowns = append(out.Unknowns, fmt.Sprintf("whether a workload of component %q waits on another component was not checked: the diagnosis was narrowed to that component; diagnose the whole Application to see it", component))
		return
	}
	producers := producersOf(applied, out.Resources)
	if len(producers) == 0 {
		return
	}
	declared := map[string]map[string]bool{}
	for _, c := range app.Spec.Components {
		declared[c.Name] = map[string]bool{}
		for _, dep := range c.DependsOn {
			declared[c.Name][dep] = true
		}
	}
	for i := range out.Resources {
		r := &out.Resources[i]
		if r.Provider != providerKubernetes || !workloadKinds[r.Kind] || r.Ready {
			continue
		}
		if *reads >= maxDiagnoseReads {
			out.Meta.Truncated = true
			out.Unknowns = append(out.Unknowns, fmt.Sprintf("%s %s/%s was not read for what it consumes: the diagnosis reached its bound of %d live reads; narrow by component", r.Kind, r.Namespace, r.Name, maxDiagnoseReads))
			continue
		}
		*reads++
		obj, err := d.readLive(ctx, r)
		if err != nil {
			out.Unknowns = append(out.Unknowns, fmt.Sprintf("%s %s/%s in cluster %s could not be read for what it consumes: %s", r.Kind, r.Namespace, r.Name, r.Cluster, redactLine(err.Error())))
			continue
		}
		out.Evidence = append(out.Evidence, fmt.Sprintf("%s %s/%s in cluster %s resourceVersion %s, read for the Secrets and ConfigMaps its pod template consumes", r.Kind, r.Namespace, r.Name, r.Cluster, obj.GetResourceVersion()))
		// one finding per consumer, producer and object, covering every key
		// the workload reads from it and whether it reads the whole object,
		// so a written host export cannot hide an unwritten port export
		type consumed struct {
			p          producer
			kind, name string
			refs       []consumedRef
		}
		groups := map[string]*consumed{}
		var order []string
		for _, ref := range consumedRefs(obj) {
			if ref.optional {
				continue
			}
			p, ok := producers[producerKey(r.Cluster, r.Namespace, ref.kind, ref.name)]
			if !ok || p.component == r.Component {
				continue
			}
			key := p.component + "\x00" + ref.kind + "\x00" + ref.name
			g := groups[key]
			if g == nil {
				g = &consumed{p: p, kind: ref.kind, name: ref.name}
				groups[key] = g
				order = append(order, key)
			}
			g.refs = append(g.refs, ref)
		}
		for _, key := range order {
			g := groups[key]
			if f, ok := consumerFinding(r, g.kind, g.name, g.refs, g.p, declared[r.Component][g.p.component], out.Resources); ok {
				out.Findings = append(out.Findings, f)
			}
		}
	}
}

// readLive reads one resource from its cluster.
func (d *deps) readLive(ctx context.Context, r *ResourceDiagnosis) (*unstructured.Unstructured, error) {
	cctx := ctx
	if r.Cluster != "" && r.Cluster != localCluster {
		cctx = multicluster.ContextWithClusterName(ctx, r.Cluster)
	}
	obj := &unstructured.Unstructured{}
	obj.SetAPIVersion(r.APIVersion)
	obj.SetKind(r.Kind)
	return obj, d.f.Client().Get(cctx, client.ObjectKey{Namespace: r.Namespace, Name: r.Name}, obj)
}

// consumerFinding states why a consumer that is not ready may be waiting on
// its producer, for everything it reads from one object: each key, and the
// whole object through envFrom or a mount. It is empty when the producer has
// done its part, or when the producer's state is not known: the consumer's
// problem is then its own, or cannot be settled from here.
func consumerFinding(r *ResourceDiagnosis, kind, name string, refs []consumedRef, p producer, declared bool, resources []ResourceDiagnosis) (Finding, bool) {
	var keys []string
	whole, anyEnv := false, false
	seenKey := map[string]bool{}
	for _, ref := range refs {
		if ref.via == viaEnv {
			anyEnv = true
		}
		if ref.key == "" {
			whole = true
			continue
		}
		if !seenKey[ref.key] {
			seenKey[ref.key] = true
			keys = append(keys, fmt.Sprintf("%q", ref.key))
		}
	}
	what := fmt.Sprintf("%s %s reads %s %q", r.Kind, r.Name, kind, name)
	switch {
	case len(keys) == 1 && !whole:
		what += " key " + keys[0]
	case len(keys) > 0 && !whole:
		what += " keys " + joinAnd(keys)
	case len(keys) > 0:
		what += " keys " + joinAnd(keys) + " and the whole object"
	default:
		what += " as a whole"
	}
	// an unwritten exporter for any key read, or for any key when the whole
	// object is read, is the most specific reason
	var unwritten []string
	seenExporter := map[string]bool{}
	note := func(e *ResourceDiagnosis) {
		if e != nil && !seenExporter[e.Name] {
			seenExporter[e.Name] = true
			unwritten = append(unwritten, fmt.Sprintf("%s %s", e.Kind, e.Name))
		}
	}
	for _, ref := range refs {
		note(p.unwrittenExporterFor(ref.key))
	}
	var why string
	switch {
	case len(unwritten) == 1:
		why = fmt.Sprintf("which %s of component %q has not written yet", unwritten[0], p.component)
	case len(unwritten) > 1:
		why = fmt.Sprintf("which %s of component %q have not written yet", joinAnd(unwritten), p.component)
	default:
		nr := notReadyOf(resources, p)
		if nr == nil {
			return Finding{}, false
		}
		why = fmt.Sprintf("which component %q fills while its %s %s is %s", p.component, nr.Kind, nr.Name, nr.State)
	}
	consequence := "the consumer started before its provider was usable and fails until the provider is ready"
	if anyEnv {
		consequence += " and, since a value read through env is fixed at container start, it restarts"
	} else {
		consequence += "; the mounted file updates in place once written"
	}
	ordering := fmt.Sprintf("%q declares no dependsOn on %q, so the workflow applied both at once; owner: %s, the Application", r.Component, p.component, ownerApplicationSpec)
	if declared {
		ordering = fmt.Sprintf("%q depends on %q, so the workflow released it when %q reported healthy; that component's health rule does not cover everything a consumer needs; owner: %s, its definition", r.Component, p.component, p.component, ownerPlatform)
	}
	return Finding{
		Severity: SeverityWarning,
		Message:  fmt.Sprintf("component %q: %s, %s; %s. %s", r.Component, what, why, consequence, ordering),
		Observed: text(r.Summary),
		Evidence: fmt.Sprintf("%s %s/%s in cluster %s", r.Kind, r.Namespace, r.Name, r.Cluster),
	}, true
}

// joinAnd joins items as "a, b and c".
func joinAnd(items []string) string {
	if len(items) <= 1 {
		return strings.Join(items, "")
	}
	return strings.Join(items[:len(items)-1], ", ") + " and " + items[len(items)-1]
}

// notReadyOf returns the first diagnosed resource of the producer's
// component, in the producer's cluster and namespace, that is known not to
// be ready, or nil. A resource whose state is unknown, after a failed read
// or the read bound, is not evidence either way.
func notReadyOf(resources []ResourceDiagnosis, p producer) *ResourceDiagnosis {
	for i := range resources {
		r := &resources[i]
		if r.Component != p.component || r.Cluster != p.cluster || r.Namespace != p.namespace {
			continue
		}
		if r.State == stateNotReady || r.State == stateHalted {
			return r
		}
	}
	return nil
}

// consumedRefs lists the Secrets and ConfigMaps a workload's pod template
// reads, in the order they appear, without duplicates: env values, envFrom
// blocks, and Secret, ConfigMap and projected volumes, in init and regular
// containers.
func consumedRefs(obj *unstructured.Unstructured) []consumedRef {
	podSpecPath := []string{"spec", "template", "spec"}
	if obj.GetKind() == "CronJob" {
		podSpecPath = []string{"spec", "jobTemplate", "spec", "template", "spec"}
	}
	podSpec, found, _ := unstructured.NestedMap(obj.Object, podSpecPath...)
	if !found {
		return nil
	}
	var refs []consumedRef
	seen := map[consumedRef]bool{}
	add := func(kind, name, key, via string, optional bool) {
		if name == "" {
			return
		}
		ref := consumedRef{kind: kind, name: name, key: key, via: via, optional: optional}
		if !seen[ref] {
			seen[ref] = true
			refs = append(refs, ref)
		}
	}
	isOptional := func(sel map[string]any) bool {
		optional, _ := sel["optional"].(bool)
		return optional
	}
	for _, field := range []string{"initContainers", "containers"} {
		containers, _ := podSpec[field].([]any)
		for _, c := range containers {
			container, _ := c.(map[string]any)
			envs, _ := container["env"].([]any)
			for _, e := range envs {
				env, _ := e.(map[string]any)
				from, _ := env["valueFrom"].(map[string]any)
				if sel, ok := from["secretKeyRef"].(map[string]any); ok {
					add(secretKind, stringAt(sel, "name"), stringAt(sel, "key"), viaEnv, isOptional(sel))
				}
				if sel, ok := from["configMapKeyRef"].(map[string]any); ok {
					add("ConfigMap", stringAt(sel, "name"), stringAt(sel, "key"), viaEnv, isOptional(sel))
				}
			}
			froms, _ := container["envFrom"].([]any)
			for _, f := range froms {
				from, _ := f.(map[string]any)
				if sel, ok := from["secretRef"].(map[string]any); ok {
					add(secretKind, stringAt(sel, "name"), "", viaEnv, isOptional(sel))
				}
				if sel, ok := from["configMapRef"].(map[string]any); ok {
					add("ConfigMap", stringAt(sel, "name"), "", viaEnv, isOptional(sel))
				}
			}
		}
	}
	// a volume that selects keys through items reads only those keys; one
	// without items reads the whole object
	addVolume := func(kind, name string, sel map[string]any) {
		items, _ := sel["items"].([]any)
		if len(items) == 0 {
			add(kind, name, "", viaVolume, isOptional(sel))
			return
		}
		for _, it := range items {
			item, _ := it.(map[string]any)
			if key := stringAt(item, "key"); key != "" {
				add(kind, name, key, viaVolume, isOptional(sel))
			}
		}
	}
	volumes, _ := podSpec["volumes"].([]any)
	for _, v := range volumes {
		volume, _ := v.(map[string]any)
		if sel, ok := volume["secret"].(map[string]any); ok {
			addVolume(secretKind, stringAt(sel, "secretName"), sel)
		}
		if sel, ok := volume["configMap"].(map[string]any); ok {
			addVolume("ConfigMap", stringAt(sel, "name"), sel)
		}
		projected, _ := volume["projected"].(map[string]any)
		sources, _ := projected["sources"].([]any)
		for _, s := range sources {
			source, _ := s.(map[string]any)
			if sel, ok := source["secret"].(map[string]any); ok {
				addVolume(secretKind, stringAt(sel, "name"), sel)
			}
			if sel, ok := source["configMap"].(map[string]any); ok {
				addVolume("ConfigMap", stringAt(sel, "name"), sel)
			}
		}
	}
	return refs
}

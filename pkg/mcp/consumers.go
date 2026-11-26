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

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/multicluster"
	querytypes "github.com/oam-dev/kubevela/pkg/utils/types"
)

// workloadKinds are the plain Kubernetes kinds whose pod template says what
// a consumer reads.
var workloadKinds = map[string]bool{"Deployment": true, "StatefulSet": true, "DaemonSet": true, "Job": true, "CronJob": true}

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

// producer is who fills a Secret or ConfigMap in this Application: an
// exporter resource of a component, or a component that manages the object
// itself.
type producer struct {
	component string
	exporter  *ResourceDiagnosis
}

func producerKey(cluster, namespace, kind, name string) string {
	return cluster + "\x00" + namespace + "\x00" + kind + "\x00" + name
}

// producersOf maps every Secret and ConfigMap this Application fills to the
// component that fills it. A component that manages the object directly is
// recorded first; an exporter writing into it takes precedence, because it
// is the more specific fact. An exporter's target lives in the namespace it
// names, else the exporter's own.
func producersOf(applied []querytypes.AppliedResource, resources []ResourceDiagnosis) map[string]producer {
	producers := map[string]producer{}
	for i := range applied {
		res := applied[i]
		if res.Kind != secretKind && res.Kind != "ConfigMap" {
			continue
		}
		producers[producerKey(clusterName(res.Cluster), res.Namespace, res.Kind, res.Name)] = producer{component: res.Component}
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
		producers[producerKey(r.Cluster, ns, r.Export.TargetKind, r.Export.TargetName)] = producer{component: r.Component, exporter: r}
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
	seen := map[string]bool{}
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
		for _, ref := range consumedRefs(obj) {
			if ref.optional {
				continue
			}
			p, ok := producers[producerKey(r.Cluster, r.Namespace, ref.kind, ref.name)]
			if !ok || p.component == r.Component {
				continue
			}
			key := r.Cluster + "\x00" + r.Namespace + "\x00" + r.Component + "\x00" + p.component + "\x00" + ref.kind + "\x00" + ref.name
			if seen[key] {
				continue
			}
			seen[key] = true
			if f, ok := consumerFinding(r, ref, p, declared[r.Component][p.component], out.Resources); ok {
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
// its producer. It is empty when the producer has done its part, or when
// the producer's state is not known: the consumer's problem is then its
// own, or cannot be settled from here.
func consumerFinding(r *ResourceDiagnosis, ref consumedRef, p producer, declared bool, resources []ResourceDiagnosis) (Finding, bool) {
	what := fmt.Sprintf("%s %s reads %s %q", r.Kind, r.Name, ref.kind, ref.name)
	if ref.key != "" {
		what += fmt.Sprintf(" key %q", ref.key)
	}
	var why string
	switch {
	case p.exporter != nil && p.exporter.Export.Written != nil && !*p.exporter.Export.Written:
		why = fmt.Sprintf("which %s %s of component %q has not written yet", p.exporter.Kind, p.exporter.Name, p.component)
	default:
		nr := notReadyOf(resources, p.component)
		if nr == nil {
			return Finding{}, false
		}
		why = fmt.Sprintf("which component %q fills while its %s %s is %s", p.component, nr.Kind, nr.Name, nr.State)
	}
	consequence := "the consumer started before its provider was usable and fails until the provider is ready"
	if ref.via == viaEnv {
		consequence += " and, since a value read through env is fixed at container start, it restarts"
	} else {
		consequence += "; the mounted file updates in place once written"
	}
	ordering := fmt.Sprintf("%q declares no dependsOn on %q, so the workflow applied both at once; the fix is in the Application spec", r.Component, p.component)
	if declared {
		ordering = fmt.Sprintf("%q depends on %q, so the workflow released it when %q reported healthy; that component's health rule does not cover everything a consumer needs, and the fix is in its definition", r.Component, p.component, p.component)
	}
	return Finding{
		Severity: SeverityWarning,
		Message:  fmt.Sprintf("component %q: %s, %s; %s. %s", r.Component, what, why, consequence, ordering),
		Observed: text(r.Summary),
		Evidence: fmt.Sprintf("%s %s/%s in cluster %s", r.Kind, r.Namespace, r.Name, r.Cluster),
	}, true
}

// notReadyOf returns the first diagnosed resource of a component that is
// known not to be ready, or nil. A resource whose state is unknown, after a
// failed read or the read bound, is not evidence either way.
func notReadyOf(resources []ResourceDiagnosis, component string) *ResourceDiagnosis {
	for i := range resources {
		r := &resources[i]
		if r.Component == component && (r.State == stateNotReady || r.State == stateHalted) {
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
	volumes, _ := podSpec["volumes"].([]any)
	for _, v := range volumes {
		volume, _ := v.(map[string]any)
		if sel, ok := volume["secret"].(map[string]any); ok {
			add(secretKind, stringAt(sel, "secretName"), "", viaVolume, isOptional(sel))
		}
		if sel, ok := volume["configMap"].(map[string]any); ok {
			add("ConfigMap", stringAt(sel, "name"), "", viaVolume, isOptional(sel))
		}
		projected, _ := volume["projected"].(map[string]any)
		sources, _ := projected["sources"].([]any)
		for _, s := range sources {
			source, _ := s.(map[string]any)
			if sel, ok := source["secret"].(map[string]any); ok {
				add(secretKind, stringAt(sel, "name"), "", viaVolume, isOptional(sel))
			}
			if sel, ok := source["configMap"].(map[string]any); ok {
				add("ConfigMap", stringAt(sel, "name"), "", viaVolume, isOptional(sel))
			}
		}
	}
	return refs
}

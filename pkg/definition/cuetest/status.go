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

package cuetest

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	k8sschema "k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/oam-dev/kubevela/pkg/appfile"
	"github.com/oam-dev/kubevela/pkg/cue/definition/health"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

// Observed is the cluster's view of the rendered objects, merged over them.
type Observed struct {
	Output  map[string]any            `json:"output,omitempty"`
	Outputs map[string]map[string]any `json:"outputs,omitempty"`
	// Traits holds each attached trait's outputs as observed, by trait type.
	Traits map[string]ObservedTrait `json:"traits,omitempty"`
}

// ObservedTrait is the cluster's view of one trait type's outputs.
type ObservedTrait struct {
	Outputs map[string]map[string]any `json:"outputs,omitempty"`
}

// StatusReport is the health, message and details of the definition under
// test and, for a component, of each attached trait by type.
type StatusReport struct {
	*health.StatusResult
	Traits map[string]*health.StatusResult
}

// Status renders the definition, then evaluates its health, message and
// details the way the controller's health check does: through the appfile
// component's template context and status evaluation, then each attached
// trait's. For a trait under test, only that trait's. The cluster is a fake
// holding every rendered object, with observed merged over it. The report is
// returned alongside any evaluation error, as the controller does.
func Status(s Subject, in Input, observed Observed) (*StatusReport, error) {
	r, err := render(s, in)
	if err != nil {
		return nil, err
	}
	if s.Kind == KindTrait {
		if observed.Output != nil {
			return nil, fmt.Errorf("observed.output: a trait's status sees only its own outputs")
		}
		observed = Observed{Traits: map[string]ObservedTrait{s.Name: {Outputs: observed.Outputs}}}
	}
	cli, err := seedCluster(r, observed)
	if err != nil {
		return nil, err
	}

	accessor := util.NewApplicationResourceNamespaceAccessor(r.namespace, "")
	var errs []string
	report := &StatusReport{Traits: map[string]*health.StatusResult{}}
	if s.Kind == KindTrait {
		report.StatusResult, errs = evalTraitStatus(r, r.comp.Traits[0], cli, accessor, "")
	} else {
		report.StatusResult, errs = evalComponentStatus(r, cli, accessor)
		for _, trait := range r.comp.Traits {
			if _, twice := report.Traits[trait.Name]; twice {
				errs = append(errs, fmt.Sprintf("traits.%s: attached twice, so its status by type is ambiguous", trait.Name))
				continue
			}
			var traitErrs []string
			report.Traits[trait.Name], traitErrs = evalTraitStatus(r, trait, cli, accessor, "traits."+trait.Name+": ")
			errs = append(errs, traitErrs...)
		}
	}
	if len(errs) > 0 {
		return report, &StatusError{Errs: errs}
	}
	return report, nil
}

func evalComponentStatus(r *rendering, cli client.Client, accessor util.NamespaceAccessor) (*health.StatusResult, []string) {
	templateContext, err := r.comp.GetTemplateContext(r.comp.Ctx, cli, accessor)
	if err != nil {
		return nil, []string{"reading rendered objects: " + err.Error()}
	}
	result, err := r.comp.EvalStatus(templateContext)
	if err != nil {
		return result, errorMessages(err)
	}
	return result, nil
}

func evalTraitStatus(r *rendering, trait *appfile.Trait, cli client.Client, accessor util.NamespaceAccessor, prefix string) (*health.StatusResult, []string) {
	templateContext, err := trait.GetTemplateContext(r.comp.Ctx, cli, accessor)
	if err != nil {
		return nil, []string{prefix + "reading rendered objects: " + err.Error()}
	}
	result, err := trait.EvalStatus(templateContext)
	if err != nil {
		var msgs []string
		for _, m := range errorMessages(err) {
			msgs = append(msgs, prefix+m)
		}
		return result, msgs
	}
	return result, nil
}

// seedCluster is a fake cluster holding every rendered object with observed
// merged over it, and what the API server sets on create filled in.
func seedCluster(r *rendering, observed Observed) (client.Client, error) {
	type seed struct {
		source       string
		obj, overlay map[string]any
	}
	seeds := []seed{{"output", r.Output, observed.Output}}
	for name := range observed.Outputs {
		if _, ok := r.Outputs[name]; !ok {
			return nil, fmt.Errorf("observed.outputs.%s: the definition renders no such output", name)
		}
	}
	for _, name := range sortedKeys(r.Outputs) {
		seeds = append(seeds, seed{"outputs." + name, r.Outputs[name], observed.Outputs[name]})
	}
	for typ, t := range observed.Traits {
		rendered, ok := r.Traits[typ]
		if !ok {
			return nil, fmt.Errorf("observed.traits.%s: no %s trait renders outputs", typ, typ)
		}
		for name := range t.Outputs {
			if _, ok := rendered.Outputs[name]; !ok {
				return nil, fmt.Errorf("observed.traits.%s.outputs.%s: the trait renders no such output", typ, name)
			}
		}
	}
	for _, typ := range sortedKeys(r.Traits) {
		outputs := r.Traits[typ].Outputs
		for _, name := range sortedKeys(outputs) {
			seeds = append(seeds, seed{"traits." + typ + ".outputs." + name, outputs[name], observed.Traits[typ].Outputs[name]})
		}
	}

	// Create rather than WithObjects: the builder panics on an object it cannot
	// register, such as one rendered without an apiVersion.
	cli := scopedClient{fake.NewClientBuilder().Build()}
	sourceOf := map[string]string{}
	for _, sd := range seeds {
		obj := &unstructured.Unstructured{Object: merge(sd.obj, sd.overlay)}
		switch {
		case clusterScoped(obj.GroupVersionKind()):
			obj.SetNamespace("")
		case obj.GetNamespace() == "":
			obj.SetNamespace(r.namespace)
		}
		if obj.GetGeneration() == 0 {
			obj.SetGeneration(1)
		}
		id := fmt.Sprintf("%s %s/%s", obj.GetKind(), obj.GetNamespace(), obj.GetName())
		if first, clash := sourceOf[obj.GetAPIVersion()+" "+id]; clash {
			return nil, fmt.Errorf("%s and %s both render %s", first, sd.source, id)
		}
		sourceOf[obj.GetAPIVersion()+" "+id] = sd.source
		if err := cli.Create(context.Background(), obj); err != nil {
			return nil, fmt.Errorf("seeding rendered %s %q: %w", obj.GetKind(), obj.GetName(), err)
		}
	}
	return cli, nil
}

// scopedClient reads a cluster-scoped kind whatever namespace a read names,
// as the API server does; the fake client files every object by namespace.
type scopedClient struct {
	client.Client
}

func (c scopedClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if clusterScoped(obj.GetObjectKind().GroupVersionKind()) {
		key.Namespace = ""
	}
	return c.Client.Get(ctx, key, obj, opts...)
}

func (c scopedClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	gvk := list.GetObjectKind().GroupVersionKind()
	gvk.Kind = strings.TrimSuffix(gvk.Kind, "List")
	if clusterScoped(gvk) {
		opts = append(opts, client.InNamespace(""))
	}
	return c.Client.List(ctx, list, opts...)
}

// clusterScoped reports whether gvk is one of Kubernetes' cluster-scoped
// built-in kinds. With no API server to ask, any other kind, a custom
// resource's included, is taken to be namespaced.
func clusterScoped(gvk k8sschema.GroupVersionKind) bool {
	return clusterScopedKinds[gvk.GroupKind()]
}

var clusterScopedKinds = map[k8sschema.GroupKind]bool{
	{Group: "", Kind: "Namespace"}:                                                    true,
	{Group: "", Kind: "Node"}:                                                         true,
	{Group: "", Kind: "PersistentVolume"}:                                             true,
	{Group: "", Kind: "ComponentStatus"}:                                              true,
	{Group: "rbac.authorization.k8s.io", Kind: "ClusterRole"}:                         true,
	{Group: "rbac.authorization.k8s.io", Kind: "ClusterRoleBinding"}:                  true,
	{Group: "apiextensions.k8s.io", Kind: "CustomResourceDefinition"}:                 true,
	{Group: "apiregistration.k8s.io", Kind: "APIService"}:                             true,
	{Group: "storage.k8s.io", Kind: "StorageClass"}:                                   true,
	{Group: "storage.k8s.io", Kind: "VolumeAttachment"}:                               true,
	{Group: "storage.k8s.io", Kind: "CSIDriver"}:                                      true,
	{Group: "storage.k8s.io", Kind: "CSINode"}:                                        true,
	{Group: "scheduling.k8s.io", Kind: "PriorityClass"}:                               true,
	{Group: "node.k8s.io", Kind: "RuntimeClass"}:                                      true,
	{Group: "networking.k8s.io", Kind: "IngressClass"}:                                true,
	{Group: "admissionregistration.k8s.io", Kind: "MutatingWebhookConfiguration"}:     true,
	{Group: "admissionregistration.k8s.io", Kind: "ValidatingWebhookConfiguration"}:   true,
	{Group: "admissionregistration.k8s.io", Kind: "ValidatingAdmissionPolicy"}:        true,
	{Group: "admissionregistration.k8s.io", Kind: "ValidatingAdmissionPolicyBinding"}: true,
	{Group: "certificates.k8s.io", Kind: "CertificateSigningRequest"}:                 true,
	{Group: "flowcontrol.apiserver.k8s.io", Kind: "FlowSchema"}:                       true,
	{Group: "flowcontrol.apiserver.k8s.io", Kind: "PriorityLevelConfiguration"}:       true,
	{Group: "internal.apiserver.k8s.io", Kind: "StorageVersion"}:                      true,
	{Group: "certificates.k8s.io", Kind: "ClusterTrustBundle"}:                        true,
	{Group: "networking.k8s.io", Kind: "IPAddress"}:                                   true,
	{Group: "networking.k8s.io", Kind: "ServiceCIDR"}:                                 true,
	{Group: "resource.k8s.io", Kind: "DeviceClass"}:                                   true,
	{Group: "resource.k8s.io", Kind: "ResourceSlice"}:                                 true,
	{Group: "storage.k8s.io", Kind: "VolumeAttributesClass"}:                          true,
	{Group: "storagemigration.k8s.io", Kind: "StorageVersionMigration"}:               true,
}

// StatusError is a failure evaluating a definition's health, message or
// details, as opposed to rendering it.
type StatusError struct {
	Errs []string
}

func (e *StatusError) Error() string {
	return strings.Join(e.Errs, "\n")
}

// errorMessages lists the messages of err, one per joined error.
func errorMessages(err error) []string {
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		var msgs []string
		for _, e := range joined.Unwrap() {
			msgs = append(msgs, errorMessages(e)...)
		}
		return msgs
	}
	return []string{err.Error()}
}

// merge returns base with overlay merged into it: maps merge recursively and
// any other overlay value replaces base's.
func merge(base, overlay map[string]any) map[string]any {
	out := make(map[string]any, len(base))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range overlay {
		ov, oIsMap := v.(map[string]any)
		bv, bIsMap := out[k].(map[string]any)
		if oIsMap && bIsMap {
			out[k] = merge(bv, ov)
			continue
		}
		out[k] = v
	}
	return out
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

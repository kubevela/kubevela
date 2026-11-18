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
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	querytypes "github.com/oam-dev/kubevela/pkg/utils/types"
	"github.com/oam-dev/kubevela/pkg/workflow/providers/query"
)

// maxEndpoints bounds the endpoints one answer lists.
const maxEndpoints = 100

// errOutsideAllowlist is what scopedReader returns for a refused read.
var errOutsideAllowlist = errors.New("outside this server's allowed namespaces")

// Endpoint is one address an Application serves on, derived the way
// vela status --endpoint derives it.
type Endpoint struct {
	Component string `json:"component"`
	Cluster   string `json:"cluster"`
	URL       string `json:"url" jsonschema:"The address as vela status --endpoint prints it."`
	Protocol  string `json:"protocol,omitempty" jsonschema:"Application protocol when known, such as http or https, else the transport protocol."`
	Host      string `json:"host"`
	Port      int    `json:"port,omitempty"`
	Path      string `json:"path,omitempty"`
	// Internal marks a ClusterIP or ExternalName Service's DNS name.
	Internal bool   `json:"internal" jsonschema:"True for a Service's in-cluster DNS name, reachable only from inside the cluster."`
	Source   string `json:"source" jsonschema:"Object the endpoint was read from, such as Service team-a/web."`
}

// endpointsOf derives the Application's endpoints from resources already
// listed for it. The generators read more than those resources: a
// cluster's nodes for a NodePort address, a route's Gateways, Seldon's
// ambassador Service. They log a failed read and move on, so every read goes
// through endpointReader, which refuses reads outside a namespace allowlist
// and records what it refused or could not read, for the unknowns.
func (d *deps) endpointsOf(ctx context.Context, cli client.Client, applied []querytypes.AppliedResource) ([]Endpoint, []string, bool) {
	reader := &endpointReader{Client: cli}
	if len(d.opts.Namespaces) > 0 {
		reader.allowed = func(ns string) bool { return ns != "" && d.namespaceAllowed(ns) == nil }
	}
	found := query.ServiceEndpointsOf(ctx, reader, applied)
	out := make([]Endpoint, 0, len(found))
	var unknowns []string
	for i := range found {
		se := &found[i]
		source := se.Ref.Kind + " " + se.Ref.Namespace + "/" + se.Ref.Name
		if se.Endpoint.Host == "" {
			if se.Ref.Kind == "Service" {
				unknowns = append(unknowns, fmt.Sprintf("%s port %d: no node address could be read for its NodePort", source, se.Endpoint.Port))
			} else {
				unknowns = append(unknowns, fmt.Sprintf("%s sets no host, so it is served on its controller's address, which this server does not read", source))
			}
			continue
		}
		protocol := strings.ToLower(string(se.Endpoint.Protocol))
		if p := se.Endpoint.AppProtocol; p != nil && *p != "" {
			protocol = *p
		}
		out = append(out, Endpoint{
			Component: se.Component,
			Cluster:   clusterName(se.Cluster),
			URL:       se.String(),
			Protocol:  protocol,
			Host:      se.Endpoint.Host,
			Port:      se.Endpoint.Port,
			Path:      se.Endpoint.Path,
			Internal:  se.Endpoint.Inner,
			Source:    source,
		})
	}
	// Addresses reachable from outside the cluster come first, across all
	// components, then by component and URL.
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Internal != out[j].Internal {
			return !out[i].Internal
		}
		if out[i].Component != out[j].Component {
			return out[i].Component < out[j].Component
		}
		return out[i].URL < out[j].URL
	})
	if refused := sortedKeys(reader.refused); len(refused) > 0 {
		unknowns = append(unknowns, "endpoints that need objects outside the allowed namespaces were not derived; not read: "+strings.Join(refused, ", "))
	}
	if failed := sortedKeys(reader.failed); len(failed) > 0 {
		unknowns = append(unknowns, "endpoints whose objects could not be read were not derived: "+strings.Join(failed, "; "))
	}
	truncated := false
	if len(out) > maxEndpoints {
		unknowns = append(unknowns, fmt.Sprintf("%d endpoints found; only the first %d are listed", len(out), maxEndpoints))
		out, truncated = out[:maxEndpoints], true
	}
	return out, unknowns, truncated
}

// endpointReader passes reads through, refusing those outside allowed when
// it is set, including cluster-scoped lists. It records what it refused and
// what failed for a reason other than the object not existing.
type endpointReader struct {
	client.Client
	allowed func(namespace string) bool

	mu      sync.Mutex
	refused map[string]bool
	failed  map[string]bool
}

func (r *endpointReader) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	what := kindOf(obj) + " " + objectName(key.Namespace, key.Name)
	if r.allowed != nil && !r.allowed(key.Namespace) {
		r.note(&r.refused, what)
		return errOutsideAllowlist
	}
	err := r.Client.Get(ctx, key, obj, opts...)
	r.noteFailure(what, err)
	return err
}

func (r *endpointReader) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	o := &client.ListOptions{}
	o.ApplyOptions(opts)
	what := strings.TrimSuffix(kindOf(list), "List") + " list " + nsOrClusterScope(o.Namespace)
	if r.allowed != nil && !r.allowed(o.Namespace) {
		r.note(&r.refused, what)
		return errOutsideAllowlist
	}
	err := r.Client.List(ctx, list, opts...)
	r.noteFailure(what, err)
	return err
}

func (r *endpointReader) noteFailure(what string, err error) {
	if err != nil && !apierrors.IsNotFound(err) {
		r.note(&r.failed, what+": "+redactLine(err.Error()))
	}
}

func (r *endpointReader) note(set *map[string]bool, what string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if *set == nil {
		*set = map[string]bool{}
	}
	(*set)[what] = true
}

func sortedKeys(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func kindOf(obj any) string {
	t := reflect.TypeOf(obj)
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t.Name()
}

func objectName(namespace, name string) string {
	if namespace == "" {
		return name
	}
	return namespace + "/" + name
}

func nsOrClusterScope(namespace string) string {
	if namespace == "" {
		return "(cluster-scoped or all namespaces)"
	}
	return "in " + namespace
}

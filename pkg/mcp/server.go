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

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"k8s.io/utils/ptr"

	velacmd "github.com/oam-dev/kubevela/pkg/cmd"
	"github.com/oam-dev/kubevela/version"
)

var errNoConfig = errors.New("no kubernetes configuration available")

// Server is a configured MCP server together with the registry that built
// it. The registry is kept so callers, tests included, can see which tools
// a given set of Options exposes without connecting a client.
type Server struct {
	srv   *sdk.Server
	opts  Options
	tools []registeredTool
}

// registeredTool records a tool's name and class after registration.
type registeredTool struct {
	Name  string
	Class ToolClass
}

// NewServer builds a server for the given factory and options. Nothing is
// served until Run or Serve is called.
func NewServer(f velacmd.Factory, opts Options) (*Server, error) {
	opts.complete()
	if opts.Transport != TransportStdio {
		return nil, fmt.Errorf("transport %q is not supported; only %q is available", opts.Transport, TransportStdio)
	}
	srv := sdk.NewServer(&sdk.Implementation{
		Name:    "vela",
		Title:   "KubeVela",
		Version: version.VelaVersion,
	}, nil)
	s := &Server{srv: srv, opts: opts}
	d := newDeps(f, opts)
	registerTools(s, d)
	return s, nil
}

// Tools lists the tools this server registered, in registration order.
func (s *Server) Tools() []registeredTool {
	out := make([]registeredTool, len(s.tools))
	copy(out, s.tools)
	return out
}

// Serve runs the server over an arbitrary transport. It blocks until the
// client disconnects or ctx is cancelled.
func (s *Server) Serve(ctx context.Context, t sdk.Transport) error {
	return s.srv.Run(ctx, t)
}

// Run serves over the configured transport. For stdio the process's stdout
// carries the protocol, so nothing else may write to it while Run is active.
func (s *Server) Run(ctx context.Context) error {
	return s.Serve(ctx, &sdk.StdioTransport{})
}

// addTool registers a tool with the SDK and records it. Read tools are
// annotated read-only so clients and gateways that honour tool annotations
// can filter on them.
func addTool[In, Out any](s *Server, class ToolClass, t *sdk.Tool, h sdk.ToolHandlerFor[In, Out]) {
	if t.Annotations == nil {
		t.Annotations = &sdk.ToolAnnotations{}
	}
	switch class {
	case ToolClassRead, ToolClassPlan:
		t.Annotations.ReadOnlyHint = true
		t.Annotations.DestructiveHint = ptr.To(false)
	case ToolClassWrite:
		t.Annotations.ReadOnlyHint = false
	}
	sdk.AddTool(s.srv, t, h)
	s.tools = append(s.tools, registeredTool{Name: t.Name, Class: class})
}

// registerTools wires every tool the options allow. Write tools are added
// only behind their flags; there is no runtime toggle a prompt could flip.
func registerTools(s *Server, d *deps) {
	addTool(s, ToolClassRead, &sdk.Tool{
		Name: "get_platform_info",
		Description: "Report the KubeVela versions and configuration this session runs against: vela CLI version, " +
			"vela-core version and health, Kubernetes server version, enabled feature gates, and any version skew. " +
			"Call once per session before other tools so explanations match the running system.",
	}, d.getPlatformInfo)

	addTool(s, ToolClassRead, &sdk.Tool{
		Name: "list_definitions",
		Description: "List the KubeVela definitions (components, traits, policies, workflow steps) available on this cluster, " +
			"with their type, namespace, version and one-line description. Use before composing an Application so only " +
			"capabilities the platform actually publishes are used.",
	}, d.listDefinitions)

	addTool(s, ToolClassRead, &sdk.Tool{
		Name: "describe_definition",
		Description: "Return one KubeVela definition's parameter schema as JSON Schema, with required fields, defaults and " +
			"description, plus its current revision. Use the returned schema as the only source of parameter names.",
	}, d.describeDefinition)

	addTool(s, ToolClassRead, &sdk.Tool{
		Name: "list_applications",
		Description: "List KubeVela Applications with phase, health, components, workflow state and whether the recorded " +
			"status is stale. Namespace narrows the list; the server's namespace restriction always applies.",
	}, d.listApplications)

	addTool(s, ToolClassRead, &sdk.Tool{
		Name: "get_application",
		Description: "Read one Application live: its declared components, traits, policies and workflow with secret-like " +
			"values removed, plus the controller's status for every service, the conditions, applied resources and the " +
			"workflow steps. Use get_managed_resource for the live objects and get_workflow for step detail.",
	}, d.getApplication)

	addTool(s, ToolClassRead, &sdk.Tool{
		Name: "get_workflow",
		Description: "Read an Application's workflow: declared steps, each step's phase, reason, message and timing, which " +
			"steps are suspended or failed, and the exact vela command that moves it forward. The server never resumes.",
	}, d.getWorkflow)

	addTool(s, ToolClassRead, &sdk.Tool{
		Name: "get_managed_resource",
		Description: "Read the objects an Application manages, from its ResourceTrackers, with the live resource tree and " +
			"health of each (Deployment, ReplicaSet, Pod and so on) in the cluster where it runs. Narrow by component, " +
			"cluster, kind and resourceName; a single match also returns the live object. Secrets return metadata only " +
			"and objects the Application does not manage are refused.",
	}, d.getManagedResource)

	addTool(s, ToolClassRead, &sdk.Tool{
		Name: "get_logs",
		Description: "Return a bounded, redacted tail of container logs for an Application's pods, found through its " +
			"managed resources. Narrow by component, cluster, pod and container; use previous for crash loops. Default " +
			"200 lines per pod, at most five pods per call.",
	}, d.getLogs)

	addTool(s, ToolClassRead, &sdk.Tool{
		Name: "explain_definition",
		Description: "Explain one KubeVela definition from its live object: what kinds its template produces, which " +
			"parameters flow into which fields, its parameter schema, health and custom status rules, trait " +
			"behaviour and restrictions, revision history, and how many Applications use it. Static inspection; " +
			"nothing is rendered or applied.",
	}, d.explainDefinition)

	addTool(s, ToolClassRead, &sdk.Tool{
		Name: "explain_application",
		Description: "Explain an Application end to end: each component with its definition, declared dependencies and " +
			"data flow, recorded status per cluster, live managed resources with unhealthy descendants and pod " +
			"phases, workflow state, recent events, and a list of findings ordered by severity, each citing the " +
			"object it came from. Start here when asked why an application is not working.",
	}, d.explainApplication)

	addTool(s, ToolClassRead, &sdk.Tool{
		Name:        "list_clusters",
		Description: "List the clusters KubeVela can deploy to, with alias, credential type, endpoint and labels. Credentials are never returned.",
	}, d.listClusters)

	addTool(s, ToolClassRead, &sdk.Tool{
		Name: "list_addons",
		Description: "List the addons enabled on this control plane with version, registry, phase, health and the clusters " +
			"they reach, read from the Applications the addons installed. Registry catalogues are not queried.",
	}, d.listAddons)

	// Plan and write tools follow in later changes; AllowWrite and AllowResume
	// are parsed now so the command's contract is stable from the first release.
}

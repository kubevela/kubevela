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
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"k8s.io/kubectl/pkg/util/i18n"
	"k8s.io/kubectl/pkg/util/templates"

	"github.com/oam-dev/kubevela/apis/types"
	velacmd "github.com/oam-dev/kubevela/pkg/cmd"
	"github.com/oam-dev/kubevela/pkg/mcp"
)

var (
	mcpLong = templates.LongDesc(i18n.T(`
		Serve KubeVela to AI agents over the Model Context Protocol (MCP).

		The server exposes the vela CLI's code paths as tools an agent can call:
		platform version and feature gates, the definitions a cluster publishes
		and their parameter schemas, Applications with their recorded status,
		workflow state, the live resource tree behind each component, a bounded,
		redacted tail of pod logs, explanations of definitions and Applications
		with every finding tied to an object, and the clusters and addons the
		control plane knows. Every response states the vela-core
		version and the time the data was read, so an agent explains the running
		system rather than a remembered one.

		Tools are grouped into read, plan and write classes. Only read and plan
		tools are available unless --allow-write is set, and resume_workflow is
		gated separately by --allow-resume because resuming bypasses the approval
		step a gated apply relies on. On the control plane the server acts with
		the caller's kubeconfig identity. Reads in other clusters go through
		cluster-gateway, which forwards that identity only when its
		ClientIdentityPenetration gate is on; results read from other clusters
		say which identity applied.

		The default transport is stdio, which is what desktop agents and coding
		assistants spawn directly. Standard output carries the protocol, so
		nothing else is printed there while the server runs.`))

	mcpExample = templates.Examples(i18n.T(`
		# Serve read-only tools over stdio for the current kubeconfig context
		vela mcp

		# Restrict every tool to two namespaces regardless of RBAC
		vela mcp --namespace team-a --namespace team-b

		# Allow write tools (gated apply) and workflow resume
		vela mcp --allow-write --allow-resume

		# Claude Code: register the server once, then use it from any session
		claude mcp add vela -- vela mcp`))
)

// MCPOption is the configuration of the vela mcp command.
type MCPOption struct {
	Transport   string
	AllowWrite  bool
	AllowResume bool
	Namespaces  []string
}

// Validate checks the flags before a server is built.
func (o *MCPOption) Validate() error {
	if o.Transport != mcp.TransportStdio {
		return fmt.Errorf("unsupported transport %q: only %q is available in this release", o.Transport, mcp.TransportStdio)
	}
	for _, ns := range o.Namespaces {
		if strings.TrimSpace(ns) == "" {
			return fmt.Errorf("--namespace must not be empty")
		}
	}
	return nil
}

// Run builds the server and serves until the client disconnects. The
// kubeconfig is loaded before the protocol starts so a missing or broken
// configuration fails the command on stderr instead of ending the session
// on the first tool call.
func (o *MCPOption) Run(f velacmd.Factory, cmd *cobra.Command) error {
	_ = f.Config()
	srv, err := mcp.NewServer(f, mcp.Options{
		Transport:   o.Transport,
		AllowWrite:  o.AllowWrite,
		AllowResume: o.AllowResume,
		Namespaces:  o.Namespaces,
	})
	if err != nil {
		return err
	}
	return srv.Run(cmd.Context())
}

// NewMCPCommand creates the `vela mcp` command.
func NewMCPCommand(f velacmd.Factory, order string) *cobra.Command {
	o := &MCPOption{Transport: mcp.TransportStdio}
	cmd := &cobra.Command{
		Use:     "mcp",
		Short:   i18n.T("Serve KubeVela to AI agents over the Model Context Protocol."),
		Long:    mcpLong,
		Example: mcpExample,
		Args:    cobra.NoArgs,
		Annotations: map[string]string{
			types.TagCommandType:  types.TypeAuxiliary,
			types.TagCommandOrder: order,
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := o.Validate(); err != nil {
				return err
			}
			return o.Run(f, cmd)
		},
	}
	cmd.Flags().StringVar(&o.Transport, "transport", o.Transport, "Transport to serve on. Only stdio is supported.")
	cmd.Flags().BoolVar(&o.AllowWrite, "allow-write", false, "Register write tools such as apply_application. Off by default.")
	cmd.Flags().BoolVar(&o.AllowResume, "allow-resume", false, "Register resume_workflow, which continues a workflow paused for approval. Off by default.")
	cmd.Flags().StringArrayVar(&o.Namespaces, "namespace", nil, "Restrict Application, resource and log reads to this namespace, including resources an Application manages elsewhere. Repeat for more than one; empty means any namespace RBAC allows. Definitions, addons and clusters in the system namespace stay readable.")
	return velacmd.NewCommandBuilder(f, cmd).WithResponsiveWriter().Build()
}

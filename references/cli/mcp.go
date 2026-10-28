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
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
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

		# Allow writes: pull-request YAML everywhere, direct writes confirmed in
		# the client only in the dev namespace
		vela mcp --allow-write --direct-namespace dev

		# Print the entry for an AI client instead of starting the server
		vela mcp config`))
)

// MCPOption is the configuration of the vela mcp command.
type MCPOption struct {
	Transport         string
	AllowWrite        bool
	AllowResume       bool
	Namespaces        []string
	DirectNamespaces  []string
	ProductionPattern string
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
	if len(o.DirectNamespaces) > 0 && !o.AllowWrite {
		return fmt.Errorf("--direct-namespace has no effect without --allow-write")
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
		Transport:         o.Transport,
		AllowWrite:        o.AllowWrite,
		AllowResume:       o.AllowResume,
		Namespaces:        o.Namespaces,
		DirectNamespaces:  o.DirectNamespaces,
		ProductionPattern: o.ProductionPattern,
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
	addServerFlags(cmd, o)
	cmd.AddCommand(NewMCPConfigCommand())
	return velacmd.NewCommandBuilder(f, cmd).WithResponsiveWriter().Build()
}

// addServerFlags registers the flags that shape what the server exposes.
// vela mcp and vela mcp config share them, so a printed entry always uses
// flags the server accepts.
func addServerFlags(cmd *cobra.Command, o *MCPOption) {
	cmd.Flags().BoolVar(&o.AllowWrite, "allow-write", false, "Register apply_application. Without --direct-namespace it only returns YAML for a pull request. Off by default.")
	cmd.Flags().StringArrayVar(&o.DirectNamespaces, "direct-namespace", nil, "Allow direct mode, confirmed by the user in their client, in this namespace. Repeat for more than one. Refused at start if it matches --production-pattern.")
	cmd.Flags().StringVar(&o.ProductionPattern, "production-pattern", mcp.DefaultProductionPattern, "Regular expression for production namespace names, which never allow direct mode.")
	cmd.Flags().BoolVar(&o.AllowResume, "allow-resume", false, "Register resume_workflow, which continues a workflow paused for approval. Off by default.")
	cmd.Flags().StringArrayVar(&o.Namespaces, "namespace", nil, "Restrict Application, resource and log reads to this namespace, including resources an Application manages elsewhere. Repeat for more than one; empty means any namespace RBAC allows. Definitions, addons and clusters in the system namespace stay readable.")
}

var (
	mcpConfigLong = templates.LongDesc(i18n.T(`
		Print the server entry an AI client needs to start vela mcp. Nothing is
		written: each client keeps its own configuration format, so the entry is
		printed either as the mcpServers JSON block most clients accept, or as the
		server's own command and arguments, which go after a client's "mcp add".

		The entry points at the absolute path of this vela binary and carries the
		server flags given here. The server uses the kubeconfig and context of the
		client process that starts it, so start the client from a shell pointed at
		the cluster you mean.`))

	mcpConfigExample = templates.Examples(i18n.T(`
		# JSON block to paste into a client's MCP configuration
		vela mcp config

		# Claude Code, read and plan tools, restricted to one namespace
		claude mcp add kubevela -- $(vela mcp config --format command --namespace team-a)

		# Allow writes, with confirmed direct mode only in dev
		vela mcp config --allow-write --direct-namespace dev`))

	// executablePath finds the running binary; tests replace it.
	executablePath = func() (string, error) {
		p, err := os.Executable()
		if err != nil {
			return "", err
		}
		return filepath.EvalSymlinks(p)
	}

	shellSafe = regexp.MustCompile(`^[A-Za-z0-9_@%+=:,./-]+$`)
)

// MCPConfigOption is the configuration of the vela mcp config command.
type MCPConfigOption struct {
	MCPOption
	Name   string
	Format string
}

// Validate refuses an entry the server would refuse to start with.
func (o *MCPConfigOption) Validate() error {
	if err := o.MCPOption.Validate(); err != nil {
		return err
	}
	if o.Format != "json" && o.Format != "command" {
		return fmt.Errorf("--format must be json or command, not %q", o.Format)
	}
	if strings.TrimSpace(o.Name) == "" {
		return fmt.Errorf("--name must not be empty")
	}
	return mcp.CheckDirectNamespaces(o.DirectNamespaces, o.ProductionPattern)
}

// ServerArgs are the arguments that start the server with these flags.
// Flags left at their defaults are omitted, so the entry stays short.
func (o *MCPConfigOption) ServerArgs() []string {
	args := []string{"mcp"}
	for _, ns := range o.Namespaces {
		args = append(args, "--namespace", ns)
	}
	if o.AllowWrite {
		args = append(args, "--allow-write")
	}
	for _, ns := range o.DirectNamespaces {
		args = append(args, "--direct-namespace", ns)
	}
	if o.ProductionPattern != "" && o.ProductionPattern != mcp.DefaultProductionPattern {
		args = append(args, "--production-pattern", o.ProductionPattern)
	}
	if o.AllowResume {
		args = append(args, "--allow-resume")
	}
	return args
}

// Run prints the entry.
func (o *MCPConfigOption) Run(cmd *cobra.Command) error {
	binary, err := executablePath()
	if err != nil {
		return fmt.Errorf("find the vela binary: %w", err)
	}
	args := o.ServerArgs()
	out := cmd.OutOrStdout()
	if o.Format == "command" {
		parts := []string{shellQuote(binary)}
		for _, a := range args {
			parts = append(parts, shellQuote(a))
		}
		_, err := fmt.Fprintln(out, strings.Join(parts, " "))
		return err
	}
	entry := map[string]any{"mcpServers": map[string]any{o.Name: map[string]any{"command": binary, "args": args}}}
	enc := json.NewEncoder(out)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	return enc.Encode(entry)
}

// shellQuote quotes a word for a POSIX shell when it needs it.
func shellQuote(s string) string {
	if shellSafe.MatchString(s) {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// NewMCPConfigCommand creates the `vela mcp config` command.
func NewMCPConfigCommand() *cobra.Command {
	o := &MCPConfigOption{MCPOption: MCPOption{Transport: mcp.TransportStdio}, Name: "kubevela", Format: "json"}
	cmd := &cobra.Command{
		Use:     "config",
		Short:   i18n.T("Print the server entry an AI client needs to start vela mcp."),
		Long:    mcpConfigLong,
		Example: mcpConfigExample,
		Args:    cobra.NoArgs,
		// The output is meant for $(...) and config files, so a refusal must
		// never print usage text into it.
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := o.Validate(); err != nil {
				return err
			}
			return o.Run(cmd)
		},
	}
	cmd.Flags().StringVar(&o.Format, "format", o.Format, "Output format: json for an mcpServers block, or command for the server command and arguments a client's mcp add takes.")
	cmd.Flags().StringVar(&o.Name, "name", o.Name, "Server name in the JSON block.")
	addServerFlags(cmd, &o.MCPOption)
	return cmd
}

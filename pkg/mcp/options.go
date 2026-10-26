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
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"

	"github.com/oam-dev/kubevela/apis/types"
)

const (
	// TransportStdio serves MCP over the process's standard input and output.
	// It is the only transport in this release; HTTP arrives with the VelaUX
	// plugin described in the KEP.
	TransportStdio = "stdio"
)

// Output bounds. They are variables so tests can lower them; nothing else
// assigns them.
var (
	// maxListItems caps list_applications and list_definitions rows.
	maxListItems = 500
	// maxObjectBytes caps a live object returned by get_managed_resource.
	maxObjectBytes = 256 << 10
	// maxTreeNodes caps the descendants listed under one managed resource.
	maxTreeNodes = 200
)

// ToolClass says what a tool may do. The class decides whether a tool is
// registered at all and lets an administrator reason about a server from
// its flags alone.
type ToolClass string

const (
	// ToolClassRead tools only read from the API server.
	ToolClassRead ToolClass = "read"
	// ToolClassPlan tools render or diff and never write.
	ToolClassPlan ToolClass = "plan"
	// ToolClassWrite tools change cluster state. They are registered only
	// when Options.AllowWrite is set.
	ToolClassWrite ToolClass = "write"
)

// Options configure a server. The zero value is a read-only stdio server
// scoped to every namespace the caller's identity can see.
type Options struct {
	// Transport selects how the server talks to its client. Only
	// TransportStdio is supported.
	Transport string
	// AllowWrite registers the write-class tools. Off by default so a fresh
	// install cannot change a cluster.
	AllowWrite bool
	// AllowResume registers resume_workflow. It is separate from AllowWrite
	// because resuming defeats the approval gate the write path relies on.
	AllowResume bool
	// Namespaces restricts every tool to these namespaces regardless of
	// RBAC. Empty means no restriction beyond RBAC.
	Namespaces []string
	// SystemNamespace is where vela-core and the built-in definitions live.
	// Defaults to types.DefaultKubeVelaNS.
	SystemNamespace string
	// ServerVersion reports the Kubernetes version behind a rest config. It
	// exists so tests can run without an API server; the default uses the
	// discovery client.
	ServerVersion func(cfg *rest.Config) (string, error)
	// PodLogs opens a pod log stream. The default uses a clientset on the
	// factory's config; tests supply their own.
	PodLogs PodLogsFunc
	// AccessReview checks what the caller may do. The default is a
	// SelfSubjectAccessReview; tests supply their own.
	AccessReview AccessReviewFunc
}

func (o *Options) complete() {
	if o.Transport == "" {
		o.Transport = TransportStdio
	}
	if o.SystemNamespace == "" {
		o.SystemNamespace = types.DefaultKubeVelaNS
	}
	if o.ServerVersion == nil {
		o.ServerVersion = discoverServerVersion
	}
	if o.AccessReview == nil {
		o.AccessReview = selfAccessReview
	}
}

func discoverServerVersion(cfg *rest.Config) (string, error) {
	if cfg == nil {
		return "", errNoConfig
	}
	dc, err := discovery.NewDiscoveryClientForConfig(cfg)
	if err != nil {
		return "", err
	}
	info, err := dc.ServerVersion()
	if err != nil {
		return "", err
	}
	return info.GitVersion, nil
}

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
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/oam-dev/kubevela/apis/types"
	velacmd "github.com/oam-dev/kubevela/pkg/cmd"
	"github.com/oam-dev/kubevela/pkg/mcp"
)

func TestNewMCPCommand(t *testing.T) {
	cmd := NewMCPCommand(velacmd.NewTestFactory(nil, nil), "3")
	require.Equal(t, "mcp", cmd.Use)
	require.Equal(t, types.TypeAuxiliary, cmd.Annotations[types.TagCommandType])
	require.Equal(t, "3", cmd.Annotations[types.TagCommandOrder])
	for _, name := range []string{"transport", "allow-write", "allow-resume", "namespace"} {
		require.NotNil(t, cmd.Flags().Lookup(name), "flag %s", name)
	}
	require.Equal(t, mcp.TransportStdio, cmd.Flags().Lookup("transport").DefValue)
	require.Equal(t, "false", cmd.Flags().Lookup("allow-write").DefValue)
	require.Equal(t, "false", cmd.Flags().Lookup("allow-resume").DefValue)
}

func TestMCPCommandRejectsBadFlags(t *testing.T) {
	cases := map[string][]string{
		"unsupported transport": {"--transport", "http"},
		"empty namespace":       {"--namespace", ""},
		"positional argument":   {"extra"},
	}
	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			cmd := NewMCPCommand(velacmd.NewTestFactory(nil, nil), "3")
			var buf bytes.Buffer
			cmd.SetOut(&buf)
			cmd.SetErr(&buf)
			cmd.SetArgs(args)
			require.Error(t, cmd.Execute())
		})
	}
}

func TestMCPOptionValidate(t *testing.T) {
	require.NoError(t, (&MCPOption{Transport: mcp.TransportStdio}).Validate())
	require.NoError(t, (&MCPOption{Transport: mcp.TransportStdio, Namespaces: []string{"a", "b"}}).Validate())
	require.Error(t, (&MCPOption{Transport: "sse"}).Validate())
	require.Error(t, (&MCPOption{Transport: mcp.TransportStdio, Namespaces: []string{" "}}).Validate())
}

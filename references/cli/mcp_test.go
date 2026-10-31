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
	"encoding/json"
	"strings"
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
	for _, name := range []string{"transport", "allow-write", "allow-resume", "namespace", "direct-namespace", "production-pattern"} {
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
		"direct without writes": {"--direct-namespace", "dev"},
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
	require.Error(t, (&MCPOption{Transport: mcp.TransportStdio, DirectNamespaces: []string{"dev"}}).Validate())
	require.NoError(t, (&MCPOption{Transport: mcp.TransportStdio, AllowWrite: true, DirectNamespaces: []string{"dev"}}).Validate())
}

// runMCPConfig runs vela mcp config with a fixed binary path.
func runMCPConfig(t *testing.T, args ...string) (string, error) {
	t.Helper()
	old := executablePath
	executablePath = func() (string, error) { return "/opt/vela tools/vela", nil }
	t.Cleanup(func() { executablePath = old })
	cmd := NewMCPCommand(velacmd.NewTestFactory(nil, nil), "3")
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs(append([]string{"config"}, args...))
	err := cmd.Execute()
	return out.String(), err
}

func TestMCPConfigJSON(t *testing.T) {
	out, err := runMCPConfig(t, "--namespace", "team-a", "--allow-write", "--direct-namespace", "dev")
	require.NoError(t, err)
	var entry struct {
		MCPServers map[string]struct {
			Command string   `json:"command"`
			Args    []string `json:"args"`
		} `json:"mcpServers"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &entry))
	server, ok := entry.MCPServers["kubevela"]
	require.True(t, ok, out)
	require.Equal(t, "/opt/vela tools/vela", server.Command)
	require.Equal(t, []string{"mcp", "--namespace", "team-a", "--allow-write", "--direct-namespace", "dev"}, server.Args)
}

func TestMCPConfigDefaultsAreReadOnlyAndShort(t *testing.T) {
	out, err := runMCPConfig(t, "--name", "vela-dev")
	require.NoError(t, err)
	require.Contains(t, out, `"vela-dev"`)
	require.Contains(t, out, `"args": [
        "mcp"
      ]`)
	require.NotContains(t, out, "--allow-write")
	require.NotContains(t, out, "--production-pattern", "the default pattern is not repeated")
}

func TestMCPConfigCommandFormatQuotes(t *testing.T) {
	out, err := runMCPConfig(t, "--format", "command", "--namespace", "team-a", "--allow-write", "--direct-namespace", "dev", "--production-pattern", "^live$")
	require.NoError(t, err)
	require.Equal(t, `'/opt/vela tools/vela' mcp --namespace team-a --allow-write --direct-namespace dev --production-pattern '^live$'`, strings.TrimSpace(out))
	require.Equal(t, `'it'\''s'`, shellQuote("it's"))
}

func TestMCPConfigRefusesWhatTheServerWould(t *testing.T) {
	cases := map[string][]string{
		"production namespace on the direct list": {"--allow-write", "--direct-namespace", "payments-prod"},
		"direct without writes":                   {"--direct-namespace", "dev"},
		"unknown format":                          {"--format", "toml"},
		"empty name":                              {"--name", " "},
		"bad production pattern":                  {"--allow-write", "--direct-namespace", "dev", "--production-pattern", "("},
	}
	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			out, err := runMCPConfig(t, args...)
			require.Error(t, err)
			require.Empty(t, out, "nothing is printed for a refused entry")
		})
	}
}

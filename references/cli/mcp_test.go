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
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/oam-dev/kubevela/apis/types"
	velacmd "github.com/oam-dev/kubevela/pkg/cmd"
	"github.com/oam-dev/kubevela/pkg/mcp"
	"github.com/oam-dev/kubevela/pkg/utils/util"
)

func TestNewMCPCommand(t *testing.T) {
	cmd := NewMCPCommand(velacmd.NewTestFactory(nil, nil), "3", util.IOStreams{})
	require.Equal(t, "mcp", cmd.Use)
	require.Equal(t, types.TypeAuxiliary, cmd.Annotations[types.TagCommandType])
	require.Equal(t, "3", cmd.Annotations[types.TagCommandOrder])
	for _, name := range []string{"transport", "allow-write", "namespace", "direct-namespace", "production-pattern"} {
		require.NotNil(t, cmd.Flags().Lookup(name), "flag %s", name)
	}
	require.Equal(t, mcp.TransportStdio, cmd.Flags().Lookup("transport").DefValue)
	require.Equal(t, "false", cmd.Flags().Lookup("allow-write").DefValue)
	require.Nil(t, cmd.Flags().Lookup("allow-resume"), "no flag for a tool that does not exist")
}

func TestMCPCommandRejectsBadFlags(t *testing.T) {
	cases := map[string][]string{
		"unsupported transport": {"--transport", "http"},
		"empty namespace":       {"--namespace", ""},
		"positional argument":   {"extra"},
		"direct without writes": {"--direct-namespace", "dev"},
		"group without user":    {"--as-group", "team-a"},
		"blank user":            {"--as", " "},
		"empty group":           {"--as", "jane", "--as-group", " "},
	}
	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			cmd := NewMCPCommand(velacmd.NewTestFactory(nil, nil), "3", util.IOStreams{})
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
	cmd := NewMCPCommand(velacmd.NewTestFactory(nil, nil), "3", util.IOStreams{})
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

func TestMCPConfigCarriesImpersonation(t *testing.T) {
	out, err := runMCPConfig(t, "--format", "command", "--as", "jane@example.com", "--as-group", "team-a", "--as-group", "dev")
	require.NoError(t, err)
	require.Equal(t, "'/opt/vela tools/vela' mcp --as jane@example.com --as-group team-a --as-group dev\n", out)
}

func TestImpersonatedFactorySendsHeaders(t *testing.T) {
	var user string
	var groups []string
	var wrapped string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, groups = r.Header.Get("Impersonate-User"), r.Header.Values("Impersonate-Group")
		wrapped = r.Header.Get("X-Caller-Wrapper")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"kind":"Namespace","apiVersion":"v1","metadata":{"name":"team-a"}}`))
	}))
	defer srv.Close()
	base := &rest.Config{Host: srv.URL}
	// The caller's own wrapper, such as the multicluster one, must still run.
	base.Wrap(func(rt http.RoundTripper) http.RoundTripper { return headerRoundTripper{rt} })

	f := impersonated(velacmd.NewTestFactory(base, nil), "jane@example.com", []string{"team-a", "dev"})
	cs, err := kubernetes.NewForConfig(f.Config())
	require.NoError(t, err)
	_, err = cs.CoreV1().Namespaces().Get(context.Background(), "team-a", metav1.GetOptions{})
	require.NoError(t, err)
	require.Equal(t, "jane@example.com", user)
	require.Equal(t, []string{"team-a", "dev"}, groups)
	require.Empty(t, base.Impersonate.UserName, "the caller's config is not changed")
	require.Equal(t, "seen", wrapped, "the caller's transport wrapper is kept")

	plain := velacmd.NewTestFactory(base, nil)
	require.Same(t, plain, impersonated(plain, "", nil), "without --as the factory is used as is")
}

// headerRoundTripper marks requests so a test can see the wrapper ran.
type headerRoundTripper struct{ rt http.RoundTripper }

func (h headerRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("X-Caller-Wrapper", "seen")
	return h.rt.RoundTrip(r)
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

// evalArgs runs line through eval in a real shell, as the documented
// registration command does, and returns the arguments the shell built.
// It runs in an empty directory so a command that escaped quoting would
// leave a file behind.
func evalArgs(t *testing.T, line string) []string {
	t.Helper()
	sh, err := exec.LookPath("sh")
	if err != nil || runtime.GOOS == "windows" {
		t.Skip("needs a POSIX shell")
	}
	dir := t.TempDir()
	cmd := exec.Command(sh, "-c", `eval "set -- $1"; printf '%s\000' "$@"`, "sh", line)
	cmd.Dir = dir
	out, err := cmd.Output()
	require.NoError(t, err)
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Empty(t, entries, "nothing in the quoted text ran")
	return strings.Split(strings.TrimSuffix(string(out), "\000"), "\000")
}

func TestMCPConfigCommandSurvivesEval(t *testing.T) {
	for _, v := range []string{"$(touch pwned)", "`touch pwned`", "a; touch pwned", "line\nbreak", "it's", "*", "~/x", "a b", "-n"} {
		require.Equal(t, []string{v}, evalArgs(t, shellQuote(v)), v)
	}
	require.Equal(t, []string{""}, evalArgs(t, shellQuote("")))

	pattern := `$(touch pwned)|'x'`
	out, err := runMCPConfig(t, "--format", "command", "--allow-write", "--direct-namespace", "dev", "--production-pattern", pattern)
	require.NoError(t, err)
	require.Equal(t,
		[]string{"/opt/vela tools/vela", "mcp", "--allow-write", "--direct-namespace", "dev", "--production-pattern", pattern},
		evalArgs(t, strings.TrimSpace(out)))
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

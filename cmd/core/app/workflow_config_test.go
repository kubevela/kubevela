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

package app

import (
	"testing"
	"time"

	"github.com/kubevela/workflow/pkg/utils/httpguard"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/require"

	"github.com/oam-dev/kubevela/cmd/core/app/config"
)

func parsedWorkflowConfig(t *testing.T, args ...string) *config.WorkflowConfig {
	t.Helper()
	c := config.NewWorkflowConfig()
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	c.AddFlags(fs)
	require.NoError(t, fs.Parse(args))
	return c
}

func TestWorkflowConfigHTTPDenyTemplateName(t *testing.T) {
	require.Equal(t, config.VelaCoreHTTPDenyConfigTemplate, config.NewWorkflowConfig().HTTPDenyConfigTemplateName, "the vela-core chart template by default")
	require.Equal(t, "custom-deny", parsedWorkflowConfig(t, "--workflow-http-deny-config-template-name=custom-deny").HTTPDenyConfigTemplateName)
	require.Empty(t, parsedWorkflowConfig(t, "--workflow-http-deny-config-template-name=").HTTPDenyConfigTemplateName, "empty turns discovery off")
}

func TestWorkflowConfigHTTPBounds(t *testing.T) {
	c := config.NewWorkflowConfig()
	require.Equal(t, 60*time.Second, c.MaxHTTPTimeout)
	require.Equal(t, int64(1<<20), c.MaxHTTPResponseBytes)

	timeout, bytes := httpguard.MaxRequestTimeout, httpguard.MaxResponseBytes
	t.Cleanup(func() { httpguard.MaxRequestTimeout, httpguard.MaxResponseBytes = timeout, bytes })
	parsedWorkflowConfig(t, "--max-workflow-http-timeout=10s", "--max-workflow-http-response-bytes=2048").SyncToWorkflowGlobals()
	require.Equal(t, 10*time.Second, httpguard.MaxRequestTimeout)
	require.Equal(t, int64(2048), httpguard.MaxResponseBytes)
}

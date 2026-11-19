/*
Copyright 2021 The KubeVela Authors.

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

package template

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func workflowstepDefDir(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	require.True(t, ok)
	// pkg/workflow/template -> repo root
	root := filepath.Clean(filepath.Join(filepath.Dir(filename), "../../.."))
	return filepath.Join(root, "vela-templates/definitions/internal/workflowstep")
}

func readWorkflowStepCue(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join(workflowstepDefDir(t), name)
	data, err := os.ReadFile(path)
	require.NoError(t, err, "read %s", path)
	return string(data)
}

func assertTimeoutPlumbing(t *testing.T, content, step string) {
	t.Helper()
	assert.Contains(t, content, `timeout?: string & =~"^(0|(([0-9]+(\\.[0-9]*)?|\\.[0-9]+)(ns|us|µs|μs|ms|s|m|h))+)$"`,
		"%s should expose optional timeout parameter with Go ParseDuration-compatible validation", step)
	assert.Contains(t, content, "Invalid values fail when the step runs",
		"%s should document runtime failure for invalid timeout values", step)
}

func assertSharedHTTPRequestOpts(t *testing.T, content, step string, mergeCount int) {
	t.Helper()
	assert.Contains(t, content, "httpRequestOpts:",
		"%s should define shared httpRequestOpts for timeout forwarding", step)
	assert.Equal(t, 1, strings.Count(content, "if parameter.timeout != _|_"),
		"%s should forward timeout in one shared httpRequestOpts block", step)
	assert.Equal(t, mergeCount, strings.Count(content, "& httpRequestOpts"),
		"%s should merge httpRequestOpts into each HTTPDo request block", step)
}

func TestRequestStepExposesHTTPTimeout(t *testing.T) {
	content := readWorkflowStepCue(t, "request.cue")
	assertTimeoutPlumbing(t, content, "request")
	assert.Contains(t, content, "if parameter.timeout != _|_",
		"request should forward timeout in its HTTPDo request block")
	assert.Contains(t, content, "timeout: parameter.timeout",
		"request should forward timeout to http.#HTTPDo as request.timeout")
	assert.Equal(t, 1, strings.Count(content, "if parameter.timeout != _|_"),
		"request should forward timeout in a single HTTPDo request block")
}

func TestWebhookStepExposesHTTPTimeout(t *testing.T) {
	content := readWorkflowStepCue(t, "webhook.cue")
	assertTimeoutPlumbing(t, content, "webhook")
	assertSharedHTTPRequestOpts(t, content, "webhook", 2)
}

func TestNotificationStepExposesHTTPTimeout(t *testing.T) {
	content := readWorkflowStepCue(t, "notification.cue")
	assertTimeoutPlumbing(t, content, "notification")
	assertSharedHTTPRequestOpts(t, content, "notification", 6)
	// Email uses email.#SendEmail and must not merge HTTP timeout opts.
	emailIdx := strings.Index(content, "email0:")
	require.Greater(t, emailIdx, 0)
	emailSection := content[emailIdx:]
	assert.NotContains(t, emailSection, "& httpRequestOpts",
		"email path should not merge HTTP timeout opts")
}

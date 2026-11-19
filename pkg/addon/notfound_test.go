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

package addon

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/google/go-github/v32/github"
	"github.com/stretchr/testify/require"
	gitlab "gitlab.com/gitlab-org/api/client-go"

	velaerrors "github.com/oam-dev/kubevela/pkg/utils/errors"
)

type codedErr struct{ code int }

func (c codedErr) Error() string   { return fmt.Sprintf("status %d", c.code) }
func (c codedErr) StatusCode() int { return c.code }

func githubErr(code int) error {
	return &github.ErrorResponse{Response: &http.Response{StatusCode: code}, Message: "gh"}
}

func gitlabErr(code int) error {
	return &gitlab.ErrorResponse{Response: &http.Response{StatusCode: code}, Message: "gl"}
}

func TestStatusOfReadsEveryBackend(t *testing.T) {
	require.Equal(t, http.StatusNotFound, statusOf(githubErr(http.StatusNotFound)))
	require.Equal(t, http.StatusForbidden, statusOf(gitlabErr(http.StatusForbidden)))
	require.Equal(t, http.StatusNotFound, statusOf(codedErr{http.StatusNotFound}))
	require.Equal(t, http.StatusNotFound, statusOf(fmt.Errorf("wrapped: %w", githubErr(http.StatusNotFound))),
		"a wrapped backend error still reports its status")
	require.Equal(t, 0, statusOf(errors.New("no status here")))
	require.Equal(t, 0, statusOf(&github.ErrorResponse{Message: "no response attached"}))
}

// A 403 is a misconfigured token. Reporting it as absence would turn a fixable
// problem into a silently empty result.
func TestAsNotFoundOnlyRewritesA404(t *testing.T) {
	require.NoError(t, asNotFound(nil, "f.yaml"))

	got := asNotFound(githubErr(http.StatusNotFound), "addons/f.yaml")
	require.ErrorIs(t, got, ErrFileNotFound, "a caller can branch on absence")
	require.Contains(t, got.Error(), "addons/f.yaml", "the message says what was missing")

	var gh *github.ErrorResponse
	require.ErrorAs(t, got, &gh,
		"the backend error is kept too: github answers 'no such repository' and "+
			"'no such file' with the same status, and dropping the cause makes them "+
			"indistinguishable afterwards")

	forbidden := gitlabErr(http.StatusForbidden)
	require.Same(t, forbidden, asNotFound(forbidden, "f.yaml"),
		"anything that is not a 404 passes through untouched")

	plain := errors.New("dial tcp: i/o timeout")
	require.Same(t, plain, asNotFound(plain, "f.yaml"))
}

// Already-a-not-found must not be wrapped twice, or the path appears in the
// message once per layer that touched it.
func TestAsNotFoundIsIdempotent(t *testing.T) {
	once := asNotFound(githubErr(http.StatusNotFound), "f.yaml")
	twice := asNotFound(once, "f.yaml")
	require.Same(t, once, twice)
}

func TestErrFileNotFoundIsTheSharedSentinel(t *testing.T) {
	require.Equal(t, velaerrors.ErrFileNotFound, ErrFileNotFound,
		"re-exported rather than redeclared, so errors.Is works across packages")
}

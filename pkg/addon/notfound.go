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

	"github.com/google/go-github/v32/github"
	gitlab "gitlab.com/gitlab-org/api/client-go"

	velaerrors "github.com/oam-dev/kubevela/pkg/utils/errors"
)

// asNotFound rewrites a client library's "no such file" as ErrFileNotFound,
// leaving every other error alone.
//
// Each backend reports absence in its own type, so callers would otherwise match
// on message text. Anything that is not a 404 passes through unchanged: a 403 is
// a misconfigured token, and reporting it as absence would turn a fixable
// problem into a silently empty result.
func asNotFound(err error, path string) error {
	if err == nil || errors.Is(err, ErrFileNotFound) {
		return err
	}
	if statusOf(err) == http.StatusNotFound {
		// Both are wrapped: the sentinel so a caller can branch on absence, and
		// the backend's own error so a log still says which 404 it was. GitHub
		// answers "no such repository" and "no such file" with the same status,
		// and dropping the cause made them indistinguishable after the fact.
		return fmt.Errorf("reading %q: %w: %w", path, ErrFileNotFound, err)
	}
	return err
}

// statusOf returns the HTTP status the backend reported, or 0.
func statusOf(err error) int {
	var gh *github.ErrorResponse
	if errors.As(err, &gh) && gh.Response != nil {
		return gh.Response.StatusCode
	}
	var gl *gitlab.ErrorResponse
	if errors.As(err, &gl) && gl.Response != nil {
		return gl.Response.StatusCode
	}
	// Anything else exposing a status, including readers that wrap their own.
	var coded interface{ StatusCode() int }
	if errors.As(err, &coded) {
		return coded.StatusCode()
	}
	return 0
}

// ErrFileNotFound is the shared sentinel, re-exported for addon callers.
var ErrFileNotFound = velaerrors.ErrFileNotFound

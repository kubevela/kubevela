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

package helm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// The registry client makes its requests without a context; bound to the pull's, each one ends
// when the pull does, so no request outlives it.
func TestPullRequestsEndWithThePull(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(30 * time.Second):
		}
	}))
	defer srv.Close()

	pullCtx, cancel := context.WithCancel(context.Background())
	client := boundToContext(&http.Client{}, pullCtx)
	time.AfterFunc(100*time.Millisecond, cancel)
	start := time.Now()
	_, err := client.Get(srv.URL)
	require.Error(t, err)
	require.Less(t, time.Since(start), 5*time.Second)
}

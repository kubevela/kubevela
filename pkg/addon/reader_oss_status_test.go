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
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-resty/resty/v2"
	"github.com/stretchr/testify/require"
)

// The OSS reader returned the response body as file content whatever the status
// was, so a 404 "succeeded" with the bucket's XML error page as the file. Any
// caller then parsed an error document as configuration.
func TestOSSReadFileHonoursTheStatusCode(t *testing.T) {
	const notFoundBody = `<?xml version="1.0" encoding="UTF-8"?>
<Error><Code>NoSuchKey</Code><Message>The specified key does not exist.</Message></Error>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "present.yaml"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("key: value\n"))
		case strings.HasSuffix(r.URL.Path, "denied.yaml"):
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte("<Error><Code>AccessDenied</Code></Error>"))
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(notFoundBody))
		}
	}))
	defer srv.Close()

	r := require.New(t)
	reader := &ossReader{bucketEndPoint: srv.URL, path: "", client: resty.New()}

	content, err := reader.ReadFile("present.yaml")
	r.NoError(err)
	r.Equal("key: value\n", content)

	// A missing file is not content, and is distinguishable from a real failure
	// so a caller can choose to carry on.
	content, err = reader.ReadFile("missing.yaml")
	r.Error(err, "a 404 must not be reported as success")
	r.True(errors.Is(err, ErrFileNotFound), "a 404 must be recognisable as not-found")
	r.NotContains(content, "NoSuchKey", "the error document must not be returned as file content")

	// Anything else is a genuine failure and must not look like absence.
	_, err = reader.ReadFile("denied.yaml")
	r.Error(err)
	r.False(errors.Is(err, ErrFileNotFound), "403 is a failure, not a missing file")
}

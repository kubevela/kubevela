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

// Package guardedhttp is the vela/http CueX package with the outbound HTTP
// guard applied. It keeps the template and the request and response types of
// github.com/kubevela/pkg/cue/cuex/providers/http and only replaces the client,
// so component, trait, policy and source templates that import vela/http get
// the same SSRF checks as workflow HTTP steps.
package guardedhttp

import (
	"context"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"strings"

	kvhttp "github.com/kubevela/pkg/cue/cuex/providers/http"
	cuexruntime "github.com/kubevela/pkg/cue/cuex/runtime"
	"github.com/kubevela/pkg/util/runtime"
	workflowfeatures "github.com/kubevela/workflow/pkg/features"
	"github.com/kubevela/workflow/pkg/utils/httpguard"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
)

// maxRedirects matches the redirect limit of net/http.
const maxRedirects = 10

func requestPolicy() httpguard.Policy {
	policy := httpguard.Current()
	if utilfeature.DefaultMutableFeatureGate.Enabled(workflowfeatures.BlockPrivateHTTPAddresses) {
		policy.BlockPrivate = true
	}
	return policy
}

// Do runs a vela/http request through the outbound HTTP guard: the deny policy
// on the URL host, the dial-time check on the resolved address, and the same
// checks on every redirect hop.
func Do(ctx context.Context, doParams *kvhttp.DoParams) (*kvhttp.DoReturns, error) {
	params := doParams.Params
	policy := requestPolicy()
	if parsed, err := neturl.Parse(params.URL); err == nil {
		if err := policy.BlockedHost(parsed.Host); err != nil {
			return nil, err
		}
	}
	req, err := http.NewRequestWithContext(ctx, params.Method, params.URL, strings.NewReader(params.Request.Body))
	if err != nil {
		return nil, err
	}
	req.Header = params.Request.Header
	req.Trailer = params.Request.Trailer

	transport := httpguard.SecureTransport(nil, policy)
	// The transport is built for this request, so release its idle
	// connections when the request ends.
	defer transport.CloseIdleConnections()
	client := &http.Client{
		Transport: transport,
		// A custom CheckRedirect replaces the net/http default, so the
		// redirect limit is enforced here too.
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
				return fmt.Errorf("stopped after %d redirects", maxRedirects)
			}
			return policy.BlockedHost(req.URL.Host)
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return &kvhttp.DoReturns{
		Returns: kvhttp.ResponseVars{
			Body:       string(b),
			Header:     resp.Header,
			Trailer:    resp.Trailer,
			StatusCode: resp.StatusCode,
		},
	}, nil
}

// Package is vela/http with the upstream template and a guarded "do".
var Package = runtime.Must(cuexruntime.NewInternalPackage(kvhttp.ProviderName,
	strings.Join(kvhttp.Package.GetTemplates(), "\n"),
	map[string]cuexruntime.ProviderFn{
		"do": cuexruntime.GenericProviderFn[kvhttp.DoParams, kvhttp.DoReturns](Do),
	}))

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

package auth

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/client-go/transport"

	"github.com/oam-dev/kubevela/pkg/multicluster"
	oamutil "github.com/oam-dev/kubevela/pkg/oam/util"
)

type testRoundTripper struct {
	Request  *http.Request
	Response *http.Response
	Err      error
}

func (rt *testRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	rt.Request = req
	return rt.Response, rt.Err
}

func TestImpersonatingRoundTripper(t *testing.T) {
	testSets := map[string]struct {
		ctxFn    func(context.Context) context.Context
		expected string
	}{
		"with service account": {
			ctxFn: func(ctx context.Context) context.Context {
				ctx = oamutil.SetServiceAccountInContext(ctx, "vela-system", "default")
				return ctx
			},
			expected: "system:serviceaccount:vela-system:default",
		},
		"without service account": {
			ctxFn: func(ctx context.Context) context.Context {
				return ctx
			},
			expected: "",
		},
		"ignore if non-local cluster request": {
			ctxFn: func(ctx context.Context) context.Context {
				ctx = multicluster.ContextWithClusterName(ctx, "test-cluster")
				ctx = oamutil.SetServiceAccountInContext(ctx, "vela-system", "default")
				return ctx
			},
			expected: "",
		},
	}
	for name, ts := range testSets {
		t.Run(name, func(t *testing.T) {
			ctx := ts.ctxFn(context.TODO())
			req, _ := http.NewRequest(http.MethodGet, "/", nil)
			req = req.WithContext(ctx)
			rt := &testRoundTripper{}
			_, err := NewImpersonatingRoundTripper(rt).RoundTrip(req)
			require.NoError(t, err)
			if ts.expected == "" {
				_, ok := rt.Request.Header[transport.ImpersonateUserHeader]
				require.False(t, ok)
				return
			}
			require.Equal(t, ts.expected, rt.Request.Header.Get(transport.ImpersonateUserHeader))
		})
	}
}

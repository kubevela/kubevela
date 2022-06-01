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
	authv1 "k8s.io/api/authentication/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"k8s.io/client-go/transport"
	featuregatetesting "k8s.io/component-base/featuregate/testing"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/features"
	"github.com/oam-dev/kubevela/pkg/oam"
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
	defer featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.AuthenticateApplication, true)()
	AuthenticationWithUser = true
	defer func() {
		AuthenticationWithUser = false
	}()
	testSets := map[string]struct {
		ctxFn         func(context.Context) context.Context
		expectedUser  string
		expectedGroup []string
	}{
		"with service account": {
			ctxFn: func(ctx context.Context) context.Context {
				app := &v1beta1.Application{}
				app.SetNamespace("vela-system")
				v1.SetMetaDataAnnotation(&app.ObjectMeta, oam.AnnotationApplicationServiceAccountName, "default")
				return ContextWithUserInfo(ctx, app)
			},
			expectedUser:  "system:serviceaccount:vela-system:default",
			expectedGroup: nil,
		},
		"without service account and app": {
			ctxFn: func(ctx context.Context) context.Context {
				return ContextWithUserInfo(ctx, nil)
			},
			expectedUser:  "",
			expectedGroup: nil,
		},
		"without service account": {
			ctxFn: func(ctx context.Context) context.Context {
				return ContextWithUserInfo(ctx, &v1beta1.Application{})
			},
			expectedUser:  AuthenticationDefaultUser,
			expectedGroup: nil,
		},
		"with user and groups": {
			ctxFn: func(ctx context.Context) context.Context {
				app := &v1beta1.Application{}
				SetUserInfoInAnnotation(&app.ObjectMeta, authv1.UserInfo{
					Username: "username",
					Groups:   []string{"kubevela:group1", "kubevela:group2"},
				})
				return ContextWithUserInfo(ctx, app)
			},
			expectedUser:  "username",
			expectedGroup: []string{"kubevela:group1", "kubevela:group2"},
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
			if ts.expectedUser == "" {
				_, ok := rt.Request.Header[transport.ImpersonateUserHeader]
				require.False(t, ok)
				return
			}
			require.Equal(t, ts.expectedUser, rt.Request.Header.Get(transport.ImpersonateUserHeader))
			require.Equal(t, ts.expectedGroup, rt.Request.Header.Values(transport.ImpersonateGroupHeader))
		})
	}
}

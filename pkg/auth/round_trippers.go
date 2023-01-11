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
	"net/http"

	utilnet "k8s.io/apimachinery/pkg/util/net"
	"k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/client-go/transport"
	"k8s.io/klog/v2"

	"github.com/oam-dev/kubevela/pkg/utils"
)

const (
	impersonateKey = "impersonate"
)

var _ utilnet.RoundTripperWrapper = &impersonatingRoundTripper{}

type impersonatingRoundTripper struct {
	rt http.RoundTripper
}

// NewImpersonatingRoundTripper will add an ImpersonateUser header to a request
// if the context has a specific user whom to act-as.
func NewImpersonatingRoundTripper(rt http.RoundTripper) http.RoundTripper {
	return &impersonatingRoundTripper{
		rt: rt,
	}
}

func (rt *impersonatingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	ctx := req.Context()
	req = req.Clone(ctx)
	userInfo, exists := request.UserFrom(ctx)
	klog.V(7).Infof("impersonation request log. path: %s method: %s user info: %+v", req.URL.String(), req.Method, userInfo)
	if exists && userInfo != nil {
		if name := userInfo.GetName(); name != "" {
			req.Header.Set(transport.ImpersonateUserHeader, name)
			for _, group := range userInfo.GetGroups() {
				req.Header.Add(transport.ImpersonateGroupHeader, group)
			}
			q := req.URL.Query()
			q.Add(impersonateKey, "true")
			req.URL.RawQuery = q.Encode()
		}
	}
	return rt.rt.RoundTrip(req)
}

func (rt *impersonatingRoundTripper) CancelRequest(req *http.Request) {
	utils.TryCancelRequest(rt.WrappedRoundTripper(), req)
}

func (rt *impersonatingRoundTripper) WrappedRoundTripper() http.RoundTripper {
	return rt.rt
}

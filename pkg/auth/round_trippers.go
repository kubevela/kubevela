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
	"k8s.io/client-go/transport"

	"github.com/oam-dev/kubevela/pkg/multicluster"
	oamutil "github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/pkg/utils"
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

	// Skip impersonation on non-local cluster requests
	if !multicluster.IsInLocalCluster(ctx) {
		return rt.rt.RoundTrip(req)
	}

	sa := oamutil.GetServiceAccountInContext(ctx)
	if sa == "" {
		return rt.rt.RoundTrip(req)
	}
	req = req.Clone(req.Context())
	req.Header.Set(transport.ImpersonateUserHeader, sa)
	return rt.rt.RoundTrip(req)
}

func (rt *impersonatingRoundTripper) CancelRequest(req *http.Request) {
	utils.TryCancelRequest(rt.WrappedRoundTripper(), req)
}

func (rt *impersonatingRoundTripper) WrappedRoundTripper() http.RoundTripper {
	return rt.rt
}

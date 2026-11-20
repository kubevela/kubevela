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

package app

import (
	"net"

	utilfeature "k8s.io/apiserver/pkg/util/feature"

	workflowfeatures "github.com/kubevela/workflow/pkg/features"
	"github.com/kubevela/workflow/pkg/utils/httpguard"
)

// alwaysDenied are the destinations no guarded outbound request of the controller's may reach,
// whatever the deny list says: the unspecified address, which reaches the host, and the local-use NAT64
// prefix, which maps onto any IPv4 address at a position its operator chooses. The guard checks
// the IPv4 address a well-known NAT64 address carries, so DNS64 to an allowed host still works.
var alwaysDenied = mustCIDRs("0.0.0.0/8", "::/128", "64:ff9b:1::/48")

// controllerHTTPPolicy is the policy the controller's guarded HTTP clients (httpguard's) apply:
// p, never reaching loopback (the controller's own metrics, health, profiling and webhook ports)
// or alwaysDenied, and denying private addresses when BlockPrivateHTTPAddresses is on.
func controllerHTTPPolicy(p httpguard.Policy) httpguard.Policy {
	p.BlockLoopback = true
	p.DenyCIDRs = append(append([]*net.IPNet{}, p.DenyCIDRs...), alwaysDenied...)
	if utilfeature.DefaultMutableFeatureGate.Enabled(workflowfeatures.BlockPrivateHTTPAddresses) {
		p.BlockPrivate = true
	}
	return p
}

func mustCIDRs(cidrs ...string) []*net.IPNet {
	out := make([]*net.IPNet, 0, len(cidrs))
	for _, c := range cidrs {
		_, n, err := net.ParseCIDR(c)
		if err != nil {
			panic(err)
		}
		out = append(out, n)
	}
	return out
}

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
	"testing"

	"github.com/stretchr/testify/require"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	featuregatetesting "k8s.io/component-base/featuregate/testing"

	workflowfeatures "github.com/kubevela/workflow/pkg/features"
	"github.com/kubevela/workflow/pkg/utils/httpguard"
)

// The controller's own outbound requests never reach the controller itself or what stands in for
// an address: loopback, the unspecified address, a well-known NAT64 address carrying either or a
// denied address, and the local-use NAT64 prefix, whose IPv4 address cannot be read off.
func TestTheControllerNeverRequestsItself(t *testing.T) {
	p := controllerHTTPPolicy(httpguard.DefaultPolicy())
	for _, addr := range []string{"127.0.0.1", "127.1.2.3", "::1", "0.0.0.0", "0.1.2.3", "::",
		"64:ff9b::7f00:1", "64:ff9b::1", "64:ff9b:1::a9fe:a9fe"} {
		require.True(t, p.Blocked(net.ParseIP(addr)), addr)
	}
	shipped := controllerHTTPPolicy(httpguard.DefaultPolicy().MergeDeny(httpguard.BuiltinDeny()))
	require.True(t, shipped.Blocked(net.ParseIP("64:ff9b::a9fe:a9fe")), "the metadata service the shipped deny list names, through NAT64")
	for _, addr := range []string{"10.0.0.1", "192.168.1.1", "8.8.8.8", "2001:4860:4860::8888", "64:ff9b::808:808"} {
		require.False(t, p.Blocked(net.ParseIP(addr)), "%s is left to the private-address setting and the deny list", addr)
	}

	featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultMutableFeatureGate, workflowfeatures.BlockPrivateHTTPAddresses, true)
	p = controllerHTTPPolicy(httpguard.DefaultPolicy())
	require.True(t, p.Blocked(net.ParseIP("10.0.0.1")), "the private-address setting still applies")
	require.True(t, p.Blocked(net.ParseIP("127.0.0.1")))
}

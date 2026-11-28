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
	"context"
	"net"

	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	workflowfeatures "github.com/kubevela/workflow/pkg/features"
	"github.com/kubevela/workflow/pkg/utils/httpguard"
)

// alwaysDenied are the destinations no guarded outbound request of the controller's may reach,
// whatever the deny list says: the unspecified address, which reaches the host, and the local-use NAT64
// prefix, which maps onto any IPv4 address at a position its operator chooses. The guard checks
// the IPv4 address a well-known NAT64 address carries, so DNS64 to an allowed host still works.
var alwaysDenied = mustCIDRs("0.0.0.0/8", "::/128", "64:ff9b:1::/48")

// controllerHTTPPolicy is the policy the controller's guarded HTTP clients apply (httpguard's,
// and common.GuardedHTTPClient's): p, never reaching loopback (the
// controller's own metrics, health, profiling and webhook ports) or alwaysDenied, and denying
// private addresses when BlockPrivateHTTPAddresses is on.
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

// loadHTTPDenyConfigMaps loads the operators' workflow HTTP deny ConfigMaps before the manager starts.
// A ConfigMap that does not parse, or a list that fails, is logged and the controller starts with the
// built-in list, as a running controller keeps its last good one; the watcher loads the ConfigMaps once
// they are fixed. Only a built-in list that does not load stops the start.
func loadHTTPDenyConfigMaps(ctx context.Context, reader client.Reader, templateName, namespace string) error {
	err := httpguard.LoadConfigMaps(ctx, reader, templateName, namespace)
	if err == nil {
		return nil
	}
	// An empty template name loads no ConfigMaps: it leaves the built-in list alone in force, and
	// fails only when that list itself does not load.
	if builtinErr := httpguard.LoadConfigMaps(ctx, reader, "", namespace); builtinErr != nil {
		return builtinErr
	}
	klog.ErrorS(err, "Failed to load workflow HTTP deny ConfigMaps; starting with the built-in list until they are fixed",
		"template", templateName, "namespace", namespace)
	return nil
}

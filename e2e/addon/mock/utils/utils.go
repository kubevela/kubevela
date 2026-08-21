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

package utils

import (
	"context"
	"fmt"
	"log"
	"net"
	"os/exec"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/yaml"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/oam-dev/kubevela/pkg/utils/common"
)

var (
	// Port is mock server's exposed port
	Port         = 9098
	velaRegistry = `
apiVersion: v1
data:
  registries: '{ "KubeVela":{ "name": "KubeVela", "oss": { "end_point": "http://REGISTRY_ADDR",
    "bucket": "" } }, "Test-Helm":{ "name": "Test-Helm", "helm": { "name":"", "password":"", "url": "http://HELM_ADDR"} } }'
kind: ConfigMap
metadata:
  name: vela-addon-registry
  namespace: vela-system
`
)

// hostGatewayAddress returns an address reachable both from this process (the
// CI runner host) and from pods running inside the "kind" Docker network, by
// resolving that network's IPv4 bridge gateway IP. The "kind" network is
// commonly dual-stack (IPv4 and IPv6 both enabled), and Docker does not
// guarantee IPAM.Config ordering, so every configured gateway is inspected
// and the first one that parses as IPv4 is used -- picking config[0]
// unconditionally can select an IPv6 entry (sometimes left with no gateway at
// all, e.g. under SLAAC), which is what silently produced an empty result
// here before. Falls back to "127.0.0.1" if the network can't be inspected or
// has no IPv4 gateway (e.g. Docker/kind not present) -- that fallback is only
// reachable from the host itself, matching this function's original behavior.
func hostGatewayAddress() string {
	out, err := exec.Command("docker", "network", "inspect", "kind", "--format", "{{range .IPAM.Config}}{{.Gateway}}{{\"\\n\"}}{{end}}").Output() // #nosec G204 -- fixed command and args, not user input
	if err != nil {
		log.Printf("could not resolve kind network gateway, falling back to 127.0.0.1 (unreachable from in-cluster pods): %v", err)
		return "127.0.0.1"
	}
	for _, line := range strings.Split(string(out), "\n") {
		gateway := strings.TrimSpace(line)
		if gateway == "" {
			continue
		}
		// Return the parsed form, not the raw string: To4 also accepts the
		// IPv4-mapped IPv6 spelling (::ffff:172.18.0.1), and splicing that into
		// http://host:port yields a malformed URL. To4().String() normalizes it to
		// dotted-quad and leaves a plain IPv4 gateway unchanged.
		if ip := net.ParseIP(gateway); ip != nil && ip.To4() != nil {
			return ip.To4().String()
		}
	}
	log.Printf("kind network inspect returned no IPv4 gateway (raw config: %q), falling back to 127.0.0.1", strings.TrimSpace(string(out)))
	return "127.0.0.1"
}

// ApplyMockServerConfig config mock server as addon registry
func ApplyMockServerConfig() error {
	args := common.Args{Schema: common.Scheme}
	k8sClient, err := args.GetClient()
	if err != nil {
		return err
	}
	ctx := context.Background()
	originCm := v1.ConfigMap{}
	cm := v1.ConfigMap{}

	hostAddr := hostGatewayAddress()
	registryCmStr := strings.ReplaceAll(velaRegistry, "REGISTRY_ADDR", fmt.Sprintf("%s:%d", hostAddr, Port))
	registryCmStr = strings.ReplaceAll(registryCmStr, "HELM_ADDR", fmt.Sprintf("%s:%d/helm", hostAddr, Port))

	err = yaml.Unmarshal([]byte(registryCmStr), &cm)
	if err != nil {
		return err
	}

	otherRegistry := cm.DeepCopy()

	err = k8sClient.Get(ctx, types.NamespacedName{Name: cm.Name, Namespace: cm.Namespace}, &originCm)
	if err != nil && apierrors.IsNotFound(err) {
		if err = k8sClient.Create(ctx, &cm); err != nil {
			return err
		}
	} else {
		cm.ResourceVersion = originCm.ResourceVersion
		if err = k8sClient.Update(ctx, &cm); err != nil {
			return err
		}
	}
	if err := k8sClient.Create(ctx, &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "test-vela"}}); err != nil {
		if !apierrors.IsAlreadyExists(err) {
			return err
		}
	}
	otherRegistry.SetNamespace("test-vela")
	if err := k8sClient.Create(ctx, otherRegistry); err != nil {
		if !apierrors.IsAlreadyExists(err) {
			return err
		}
	}
	return nil
}

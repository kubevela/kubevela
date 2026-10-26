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

package mcp

import (
	"context"
	"fmt"
	"strings"

	goversion "github.com/hashicorp/go-version"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/version"
)

const (
	// controllerNameLabel is set on the vela-core Deployment by the chart.
	controllerNameLabel = "controller.oam.dev/name"
	controllerNameValue = "vela-core"
	// chartVersionLabel is the standard label the chart stamps with its appVersion.
	chartVersionLabel = "app.kubernetes.io/version"
	featureGatesFlag  = "--feature-gates="
	unknownVersion    = "unknown"
	localCluster      = "local"
	// gatewayNameSuffix is how the chart names the cluster-gateway Deployment.
	gatewayNameSuffix = "cluster-gateway"
	// identityPenetrationGate is the cluster-gateway feature gate that makes
	// it impersonate the caller in managed clusters.
	identityPenetrationGate = "ClientIdentityPenetration"
)

// platformInfo is what get_platform_info returns and what the envelope is
// built from. The two share one lookup so they cannot disagree.
type platformInfo struct {
	VelaCLI      string            `json:"velaCLI"`
	VelaCore     string            `json:"velaCore"`
	GitRevision  string            `json:"gitRevision"`
	Kubernetes   map[string]string `json:"kubernetes"`
	FeatureGates map[string]bool   `json:"featureGates,omitempty"`
	Controller   controllerInfo    `json:"controller"`
	// Gateway describes how reads in other clusters are authenticated.
	Gateway  gatewayInfo `json:"clusterGateway"`
	Warnings []string    `json:"warnings,omitempty"`
}

// gatewayInfo records whether cluster-gateway forwards the caller's
// identity to managed clusters. When ClientIdentityPenetration is off, a
// remote read runs as the credential stored for that cluster, which is
// usually an administrator; the server says so on every remote read.
type gatewayInfo struct {
	Found bool `json:"found"`
	// IdentityPenetration is true when the gateway impersonates the caller
	// in the managed cluster, false when it does not, nil when unknown.
	IdentityPenetration *bool `json:"identityPenetration"`
}

// controllerInfo describes the vela-core Deployment as found.
type controllerInfo struct {
	Namespace     string `json:"namespace"`
	Name          string `json:"name,omitempty"`
	Image         string `json:"image,omitempty"`
	Replicas      int32  `json:"replicas"`
	ReadyReplicas int32  `json:"readyReplicas"`
	Found         bool   `json:"found"`
}

// GetPlatformInfoInput optionally names the namespace the user is working in.
type GetPlatformInfoInput struct {
	Namespace string `json:"namespace,omitempty" jsonschema:"Namespace the user is about to work in. When set, the result reports how that namespace delivers workloads today."`
}

// GetPlatformInfoOutput is the tool's structured result.
type GetPlatformInfoOutput struct {
	platformInfo
	// Practice is set when a namespace was given.
	Practice *DeliveryPractice `json:"practice,omitempty" jsonschema:"How the namespace delivers workloads today. Follow it: keep existing workloads in their form and match it for new work unless the user asks otherwise."`
	Meta     Meta              `json:"meta"`
}

// getPlatformInfo never fails on a partial lookup: whatever could be read is
// returned and the lookup error is repeated in the envelope's warnings.
func (d *deps) getPlatformInfo(ctx context.Context, _ *sdk.CallToolRequest, in GetPlatformInfoInput) (*sdk.CallToolResult, GetPlatformInfoOutput, error) {
	info, _ := d.cachedPlatform(ctx)
	out := GetPlatformInfoOutput{platformInfo: info, Meta: d.meta(ctx)}
	if in.Namespace != "" {
		practice, warnings, err := d.deliveryPractice(ctx, in.Namespace)
		if err != nil {
			return nil, out, err
		}
		out.Practice = practice
		out.Meta.Warnings = append(out.Meta.Warnings, warnings...)
		out.Meta.Truncated = out.Meta.Truncated || practice.Truncated
	}
	return nil, out, nil
}

// readPlatform gathers versions and gates. Each part degrades independently:
// a missing Deployment or an unreachable discovery endpoint produces an
// "unknown" with a warning, not a failure, and the first error is returned
// so the caller can surface it.
func (d *deps) readPlatform(ctx context.Context) (platformInfo, error) {
	info := platformInfo{
		VelaCLI:     version.VelaVersion,
		VelaCore:    unknownVersion,
		GitRevision: version.GitRevision,
		Kubernetes:  map[string]string{localCluster: unknownVersion},
		Controller:  controllerInfo{Namespace: d.opts.SystemNamespace},
	}
	var firstErr error
	note := func(err error) {
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}

	if kv, err := d.opts.ServerVersion(d.f.Config()); err != nil {
		note(fmt.Errorf("kubernetes version: %w", err))
	} else {
		info.Kubernetes[localCluster] = kv
	}

	deploy, err := d.findController(ctx)
	switch {
	case err != nil:
		note(fmt.Errorf("vela-core deployment: %w", err))
	case deploy == nil:
		info.Warnings = append(info.Warnings, fmt.Sprintf("no vela-core deployment found in namespace %q", d.opts.SystemNamespace))
	default:
		info.Controller = describeController(deploy)
		info.VelaCore = controllerVersion(deploy)
		info.FeatureGates = parseFeatureGates(deploy)
		if info.Controller.ReadyReplicas == 0 {
			info.Warnings = append(info.Warnings, "vela-core has no ready replicas")
		}
	}

	if w := versionSkew(info.VelaCLI, info.VelaCore); w != "" {
		info.Warnings = append(info.Warnings, w)
	}

	gateway, err := d.findGateway(ctx)
	switch {
	case err != nil:
		note(fmt.Errorf("cluster-gateway deployment: %w", err))
	case gateway != nil:
		info.Gateway.Found = true
		if v, ok := parseFeatureGates(gateway)[identityPenetrationGate]; ok {
			info.Gateway.IdentityPenetration = &v
		} else {
			off := false
			info.Gateway.IdentityPenetration = &off
		}
	}
	return info, firstErr
}

// findGateway returns the cluster-gateway Deployment in the system
// namespace, found by the name suffix the chart gives it, or nil.
func (d *deps) findGateway(ctx context.Context) (*appsv1.Deployment, error) {
	cli := d.f.Client()
	if cli == nil {
		return nil, errNoConfig
	}
	var list appsv1.DeploymentList
	if err := cli.List(ctx, &list, client.InNamespace(d.opts.SystemNamespace)); err != nil {
		return nil, err
	}
	for i := range list.Items {
		if strings.HasSuffix(list.Items[i].Name, gatewayNameSuffix) {
			return &list.Items[i], nil
		}
	}
	return nil, nil
}

// remoteReadWarning is appended to the envelope when a result includes
// data read from a cluster other than the control plane and the gateway
// does not forward the caller's identity. It is the honest version of
// "never elevates": the hub call is the caller's, the remote one may not be.
func (d *deps) remoteReadWarning(ctx context.Context, clusters map[string]bool) string {
	remote := false
	for c := range clusters {
		if c != "" && c != localCluster {
			remote = true
			break
		}
	}
	if !remote {
		return ""
	}
	info, _ := d.cachedPlatform(ctx)
	switch {
	case info.Gateway.IdentityPenetration != nil && *info.Gateway.IdentityPenetration:
		return ""
	case info.Gateway.Found:
		return "remote cluster reads ran with the credential stored for each cluster, not your identity: cluster-gateway has " + identityPenetrationGate + " disabled"
	default:
		return "remote cluster reads may have run with the credential stored for each cluster: cluster-gateway configuration could not be read"
	}
}

// findController returns the vela-core Deployment in the system namespace,
// or nil when there is none. The chart labels it controller.oam.dev/name=vela-core.
func (d *deps) findController(ctx context.Context) (*appsv1.Deployment, error) {
	cli := d.f.Client()
	if cli == nil {
		return nil, errNoConfig
	}
	var list appsv1.DeploymentList
	if err := cli.List(ctx, &list,
		client.InNamespace(d.opts.SystemNamespace),
		client.MatchingLabels{controllerNameLabel: controllerNameValue},
	); err != nil {
		return nil, err
	}
	if len(list.Items) == 0 {
		return nil, nil
	}
	return &list.Items[0], nil
}

func describeController(deploy *appsv1.Deployment) controllerInfo {
	info := controllerInfo{
		Namespace:     deploy.Namespace,
		Name:          deploy.Name,
		Found:         true,
		ReadyReplicas: deploy.Status.ReadyReplicas,
	}
	if deploy.Spec.Replicas != nil {
		info.Replicas = *deploy.Spec.Replicas
	}
	if c := mainContainer(deploy); c != nil {
		info.Image = c.Image
	}
	return info
}

// controllerVersion prefers the chart's version label and falls back to the
// image tag, which is what a hand-built install carries.
func controllerVersion(deploy *appsv1.Deployment) string {
	if v := deploy.Labels[chartVersionLabel]; v != "" {
		return v
	}
	if c := mainContainer(deploy); c != nil {
		if i := strings.LastIndex(c.Image, ":"); i > 0 && !strings.Contains(c.Image[i:], "/") {
			return c.Image[i+1:]
		}
	}
	return unknownVersion
}

// parseFeatureGates reads every --feature-gates argument. The chart passes
// one flag per gate, but a single comma-separated flag is also accepted.
func parseFeatureGates(deploy *appsv1.Deployment) map[string]bool {
	c := mainContainer(deploy)
	if c == nil {
		return nil
	}
	gates := map[string]bool{}
	for _, arg := range append(append([]string{}, c.Args...), c.Command...) {
		if !strings.HasPrefix(arg, featureGatesFlag) {
			continue
		}
		for _, kv := range strings.Split(strings.TrimPrefix(arg, featureGatesFlag), ",") {
			k, v, ok := strings.Cut(strings.TrimSpace(kv), "=")
			if !ok || k == "" {
				continue
			}
			gates[k] = strings.EqualFold(strings.TrimSpace(v), "true")
		}
	}
	if len(gates) == 0 {
		return nil
	}
	return gates
}

// mainContainer returns the container that runs the controller: the one
// whose image or name says so, else the first.
func mainContainer(deploy *appsv1.Deployment) *corev1.Container {
	cs := deploy.Spec.Template.Spec.Containers
	if len(cs) == 0 {
		return nil
	}
	for i := range cs {
		if strings.Contains(cs[i].Image, controllerNameValue) || cs[i].Name == "kubevela" {
			return &cs[i]
		}
	}
	return &cs[0]
}

// versionSkew returns a warning when the CLI and controller differ in minor
// version. Patch differences are normal; minor differences change behaviour.
func versionSkew(cli, core string) string {
	cv, err1 := goversion.NewVersion(strings.TrimPrefix(cli, "v"))
	kv, err2 := goversion.NewVersion(strings.TrimPrefix(core, "v"))
	if err1 != nil || err2 != nil {
		return ""
	}
	cs, ks := cv.Segments(), kv.Segments()
	if len(cs) < 2 || len(ks) < 2 {
		return ""
	}
	if cs[0] != ks[0] || cs[1] != ks[1] {
		return fmt.Sprintf("version skew: vela CLI %s and vela-core %s differ in minor version; explanations follow the controller", cli, core)
	}
	return ""
}

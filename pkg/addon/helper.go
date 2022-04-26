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

package addon

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	"k8s.io/client-go/discovery"
	"k8s.io/klog/v2"

	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	commontypes "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/multicluster"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/utils/apply"
)

const (
	// disabled indicates the addon is disabled
	disabled = "disabled"
	// enabled indicates the addon is enabled
	enabled = "enabled"
	// enabling indicates the addon is enabling
	enabling = "enabling"
	// disabling indicates the addon related app is deleting
	disabling = "disabling"
	// suspend indicates the addon related app is suspend
	suspend = "suspend"
)

// EnableAddon will enable addon with dependency check, source is where addon from.
func EnableAddon(ctx context.Context, name string, version string, cli client.Client, discoveryClient *discovery.DiscoveryClient, apply apply.Applicator, config *rest.Config, r Registry, args map[string]interface{}, cache *Cache) error {
	h := NewAddonInstaller(ctx, cli, discoveryClient, apply, config, &r, args, cache)
	pkg, err := h.loadInstallPackage(name, version)
	if err != nil {
		return err
	}
	err = h.enableAddon(pkg)
	if err != nil {
		return err
	}
	return nil
}

// DisableAddon will disable addon from cluster.
func DisableAddon(ctx context.Context, cli client.Client, name string, config *rest.Config, force bool) error {
	app, err := FetchAddonRelatedApp(ctx, cli, name)
	// if app not exist, report error
	if err != nil {
		return err
	}

	if !force {
		var usingAddonApp []v1beta1.Application
		usingAddonApp, err = checkAddonHasBeenUsed(ctx, cli, name, *app, config)
		if err != nil {
			return err
		}
		if len(usingAddonApp) != 0 {
			return fmt.Errorf(fmt.Sprintf("%s please delete them first", usingAppsInfo(usingAddonApp)))
		}
	}

	if err := cli.Delete(ctx, app); err != nil {
		return err
	}
	return nil
}

// EnableAddonByLocalDir enable an addon from local dir
func EnableAddonByLocalDir(ctx context.Context, name string, dir string, cli client.Client, dc *discovery.DiscoveryClient, applicator apply.Applicator, config *rest.Config, args map[string]interface{}) error {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	r := localReader{dir: absDir, name: name}
	metas, err := r.ListAddonMeta()
	if err != nil {
		return err
	}
	meta := metas[r.name]
	UIData, err := GetUIDataFromReader(r, &meta, UIMetaOptions)
	if err != nil {
		return err
	}
	pkg, err := GetInstallPackageFromReader(r, &meta, UIData)
	if err != nil {
		return err
	}
	h := NewAddonInstaller(ctx, cli, dc, applicator, config, &Registry{Name: LocalAddonRegistryName}, args, nil)
	needEnableAddonNames, err := h.checkDependency(pkg)
	if err != nil {
		return err
	}
	if len(needEnableAddonNames) > 0 {
		return fmt.Errorf("you must first enable dependencies: %v", needEnableAddonNames)
	}

	err = h.enableAddon(pkg)
	if err != nil {
		return err
	}
	return nil
}

// GetAddonStatus is genrall func for cli and apiServer get addon status
func GetAddonStatus(ctx context.Context, cli client.Client, name string) (Status, error) {
	app, err := FetchAddonRelatedApp(ctx, cli, name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return Status{AddonPhase: disabled, AppStatus: nil}, nil
		}
		return Status{}, err
	}
	var clusters = make(map[string]map[string]interface{})
	for _, r := range app.Status.AppliedResources {
		if r.Cluster == "" {
			r.Cluster = multicluster.ClusterLocalName
		}
		// TODO(wonderflow): we should collect all the necessary information as observability, currently we only collect cluster name
		clusters[r.Cluster] = make(map[string]interface{})
	}

	if app.Status.Workflow != nil && app.Status.Workflow.Suspend {
		return Status{AddonPhase: suspend, AppStatus: &app.Status, Clusters: clusters, InstalledVersion: app.GetLabels()[oam.LabelAddonVersion]}, nil
	}
	switch app.Status.Phase {
	case commontypes.ApplicationRunning:

		if name == ObservabilityAddon {
			// TODO(wonderflow): this is a hack Implementation and need be fixed in a unified way
			var (
				sec    v1.Secret
				domain string
			)
			if err = cli.Get(ctx, client.ObjectKey{Namespace: types.DefaultKubeVelaNS, Name: Convert2SecName(name)}, &sec); err != nil {
				klog.ErrorS(err, "failed to get observability secret")
				return Status{AddonPhase: enabling, AppStatus: &app.Status, Clusters: clusters}, nil
			}

			if v, ok := sec.Data[ObservabilityAddonDomainArg]; ok {
				domain = string(v)
			}
			observability, err := GetObservabilityAccessibilityInfo(ctx, cli, domain)
			if err != nil {
				klog.ErrorS(err, "failed to get observability accessibility info")
				return Status{AddonPhase: enabling, AppStatus: &app.Status, Clusters: clusters}, nil
			}

			for _, o := range observability {
				var access = fmt.Sprintf("No loadBalancer found, visiting by using 'vela port-forward %s", ObservabilityAddon)
				if o.LoadBalancerIP != "" {
					access = fmt.Sprintf("Visiting URL: %s, IP: %s", o.Domain, o.LoadBalancerIP)
				}
				clusters[o.Cluster] = map[string]interface{}{
					"domain":            o.Domain,
					"loadBalancerIP":    o.LoadBalancerIP,
					"access":            access,
					"serviceExternalIP": o.ServiceExternalIP,
				}
			}
			return Status{AddonPhase: enabled, AppStatus: &app.Status, Clusters: clusters}, nil
		}
		return Status{AddonPhase: enabled, AppStatus: &app.Status, InstalledVersion: app.GetLabels()[oam.LabelAddonVersion], Clusters: clusters}, nil
	case commontypes.ApplicationDeleting:
		return Status{AddonPhase: disabling, AppStatus: &app.Status, Clusters: clusters}, nil
	default:
		return Status{AddonPhase: enabling, AppStatus: &app.Status, InstalledVersion: app.GetLabels()[oam.LabelAddonVersion], Clusters: clusters}, nil
	}
}

// GetObservabilityAccessibilityInfo will get the accessibility info of addon in local cluster and multiple clusters
func GetObservabilityAccessibilityInfo(ctx context.Context, k8sClient client.Client, domain string) ([]ObservabilityEnvironment, error) {
	domains, err := allocateDomainForAddon(ctx, k8sClient)
	if err != nil {
		return nil, err
	}

	obj := new(unstructured.Unstructured)
	obj.SetKind("Service")
	obj.SetAPIVersion("v1")
	key := client.ObjectKeyFromObject(obj)
	key.Namespace = types.DefaultKubeVelaNS
	key.Name = ObservabilityAddonEndpointComponent
	for i, d := range domains {
		if err != nil {
			return nil, err
		}
		readCtx := multicluster.ContextWithClusterName(ctx, d.Cluster)
		if err := k8sClient.Get(readCtx, key, obj); err != nil {
			return nil, err
		}
		var svc v1.Service
		data, err := obj.MarshalJSON()
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal(data, &svc); err != nil {
			return nil, err
		}
		if svc.Status.LoadBalancer.Ingress != nil && len(svc.Status.LoadBalancer.Ingress) == 1 {
			domains[i].ServiceExternalIP = svc.Status.LoadBalancer.Ingress[0].IP
		}
	}
	// set domain for the cluster if there is no child clusters
	if len(domains) == 0 {
		var svc v1.Service
		if err := k8sClient.Get(ctx, client.ObjectKey{Name: ObservabilityAddonEndpointComponent, Namespace: types.DefaultKubeVelaNS}, &svc); err != nil {
			return nil, err
		}
		if svc.Status.LoadBalancer.Ingress != nil && len(svc.Status.LoadBalancer.Ingress) == 1 {
			domains = []ObservabilityEnvironment{
				{
					ServiceExternalIP: svc.Status.LoadBalancer.Ingress[0].IP,
				},
			}
		}
	}
	return domains, nil
}

// Status contain addon phase and related app status
type Status struct {
	AddonPhase string
	AppStatus  *commontypes.AppStatus
	// the status of multiple clusters
	Clusters         map[string]map[string]interface{} `json:"clusters,omitempty"`
	InstalledVersion string
}

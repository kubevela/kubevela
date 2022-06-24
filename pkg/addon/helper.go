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
	addonutil "github.com/oam-dev/kubevela/pkg/utils/addon"
	"github.com/oam-dev/kubevela/pkg/utils/apply"
	"github.com/oam-dev/kubevela/pkg/utils/common"
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
func EnableAddon(ctx context.Context, name string, version string, cli client.Client, discoveryClient *discovery.DiscoveryClient, apply apply.Applicator, config *rest.Config, r Registry, args map[string]interface{}, cache *Cache, opts ...InstallOption) error {
	h := NewAddonInstaller(ctx, cli, discoveryClient, apply, config, &r, args, cache, opts...)
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
func EnableAddonByLocalDir(ctx context.Context, name string, dir string, cli client.Client, dc *discovery.DiscoveryClient, applicator apply.Applicator, config *rest.Config, args map[string]interface{}, opts ...InstallOption) error {
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
	h := NewAddonInstaller(ctx, cli, dc, applicator, config, &Registry{Name: LocalAddonRegistryName}, args, nil, opts...)
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

// GetAddonStatus is general func for cli and apiServer get addon status
func GetAddonStatus(ctx context.Context, cli client.Client, name string) (Status, error) {
	var addonStatus Status

	app, err := FetchAddonRelatedApp(ctx, cli, name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			addonStatus.AddonPhase = disabled
			return addonStatus, nil
		}
		return addonStatus, err
	}
	labels := app.GetLabels()
	addonStatus.AppStatus = &app.Status
	addonStatus.InstalledVersion = labels[oam.LabelAddonVersion]
	addonStatus.InstalledRegistry = labels[oam.LabelAddonRegistry]

	var clusters = make(map[string]map[string]interface{})
	for _, r := range app.Status.AppliedResources {
		if r.Cluster == "" {
			r.Cluster = multicluster.ClusterLocalName
		}
		// TODO(wonderflow): we should collect all the necessary information as observability, currently we only collect cluster name
		clusters[r.Cluster] = make(map[string]interface{})
	}
	addonStatus.Clusters = clusters

	if app.Status.Workflow != nil && app.Status.Workflow.Suspend {
		addonStatus.AddonPhase = suspend
		return addonStatus, nil
	}

	// Get addon parameters
	var sec v1.Secret
	err = cli.Get(ctx, client.ObjectKey{Namespace: types.DefaultKubeVelaNS, Name: addonutil.Convert2SecName(name)}, &sec)
	if err != nil {
		// Not found error can be ignored. Others can't.
		if !apierrors.IsNotFound(err) {
			return addonStatus, err
		}
	} else {
		// Although normally `else` is not preferred, we must use `else` here.
		args, err := FetchArgsFromSecret(&sec)
		if err != nil {
			return addonStatus, err
		}
		addonStatus.Parameters = args
	}

	switch app.Status.Phase {
	case commontypes.ApplicationRunning:
		addonStatus.AddonPhase = enabled
		if name == ObservabilityAddon {
			// TODO(wonderflow): this is a hack Implementation and need be fixed in a unified way
			var (
				sec    v1.Secret
				domain string
			)
			if err = cli.Get(ctx, client.ObjectKey{Namespace: types.DefaultKubeVelaNS, Name: addonutil.Convert2SecName(name)}, &sec); err != nil {
				klog.ErrorS(err, "failed to get observability secret")
				addonStatus.AddonPhase = enabling
				addonStatus.InstalledVersion = ""
				return addonStatus, nil
			}

			if v, ok := sec.Data[ObservabilityAddonDomainArg]; ok {
				domain = string(v)
			}
			observability, err := GetObservabilityAccessibilityInfo(ctx, cli, domain)
			if err != nil {
				klog.ErrorS(err, "failed to get observability accessibility info")
				addonStatus.AddonPhase = enabling
				addonStatus.InstalledVersion = ""
				return addonStatus, nil
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

			return addonStatus, nil
		}
		return addonStatus, nil
	case commontypes.ApplicationDeleting:
		addonStatus.AddonPhase = disabling
		return addonStatus, nil
	default:
		addonStatus.AddonPhase = enabling
		return addonStatus, nil
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

// FindWholeAddonPackagesFromRegistry find addons' WholeInstallPackage from registries, empty registryName indicates matching all
func FindWholeAddonPackagesFromRegistry(ctx context.Context, k8sClient client.Client, addonNames []string, registryNames []string) ([]*WholeAddonPackage, error) {
	var addons []*WholeAddonPackage
	var registries []Registry

	if len(addonNames) == 0 {
		return nil, fmt.Errorf("no addon name specified")
	}

	registryDataStore := NewRegistryDataStore(k8sClient)

	// Find matched registries
	if len(registryNames) == 0 {
		// Empty registryNames will match all registries
		regs, err := registryDataStore.ListRegistries(ctx)
		if err != nil {
			return nil, err
		}
		registries = regs
	} else {
		// Only match specified registries
		for _, registryName := range registryNames {
			r, err := registryDataStore.GetRegistry(ctx, registryName)
			if err != nil {
				continue
			}
			registries = append(registries, r)
		}
	}

	if len(registries) == 0 {
		return nil, ErrRegistryNotExist
	}

	// Found addons, for deduplication purposes
	foundAddons := make(map[string]bool)
	merge := func(addon *WholeAddonPackage) {
		if _, ok := foundAddons[addon.Name]; !ok {
			foundAddons[addon.Name] = true
		}
		addons = append(addons, addon)
	}

	// Find matched addons in registries
	for _, r := range registries {
		if IsVersionRegistry(r) {
			vr := BuildVersionedRegistry(r.Name, r.Helm.URL, &common.HTTPOption{Username: r.Helm.Username, Password: r.Helm.Password})
			for _, addonName := range addonNames {
				wholePackage, err := vr.GetDetailedAddon(ctx, addonName, "")
				if err != nil {
					continue
				}
				merge(wholePackage)
			}
		} else {
			meta, err := r.ListAddonMeta()
			if err != nil {
				continue
			}

			for _, addonName := range addonNames {
				sourceMeta, ok := meta[addonName]
				if !ok {
					continue
				}
				uiData, err := r.GetUIData(&sourceMeta, CLIMetaOptions)
				if err != nil {
					continue
				}
				installPackage, err := r.GetInstallPackage(&sourceMeta, uiData)
				if err != nil {
					continue
				}
				// Combine UIData and InstallPackage into WholeAddonPackage
				wholePackage := &WholeAddonPackage{
					InstallPackage:    *installPackage,
					APISchema:         uiData.APISchema,
					Detail:            uiData.Detail,
					AvailableVersions: uiData.AvailableVersions,
					RegistryName:      uiData.RegistryName,
				}
				merge(wholePackage)
			}
		}
	}

	if len(addons) == 0 {
		return nil, ErrNotExist
	}

	return addons, nil
}

// Status contain addon phase and related app status
type Status struct {
	AddonPhase string
	AppStatus  *commontypes.AppStatus
	// the status of multiple clusters
	Clusters         map[string]map[string]interface{} `json:"clusters,omitempty"`
	InstalledVersion string
	Parameters       map[string]interface{}
	// Where the addon is from. Can be empty if not installed.
	InstalledRegistry string
}

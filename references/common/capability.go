package common

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/oam-dev/kubevela/pkg/utils/common"

	corev1 "k8s.io/api/core/v1"

	"github.com/ghodss/yaml"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/pkg/utils/helm"
	"github.com/oam-dev/kubevela/pkg/utils/system"
	cmdutil "github.com/oam-dev/kubevela/pkg/utils/util"
	"github.com/oam-dev/kubevela/references/apiserver/apis"
	"github.com/oam-dev/kubevela/references/plugins"
)

// AddCapabilityCenter will add a cap center
func AddCapabilityCenter(capName, capURL, capToken string) error {
	repos, err := plugins.LoadRepos()
	if err != nil {
		return err
	}
	config := &plugins.CapCenterConfig{
		Name:    capName,
		Address: capURL,
		Token:   capToken,
	}
	var updated bool
	for idx, r := range repos {
		if r.Name == config.Name {
			repos[idx] = *config
			updated = true
			break
		}
	}
	if !updated {
		repos = append(repos, *config)
	}
	if err = plugins.StoreRepos(repos); err != nil {
		return err
	}
	return SyncCapabilityFromCenter(capName, capURL, capToken)
}

// SyncCapabilityFromCenter will sync all capabilities from center
func SyncCapabilityFromCenter(capName, capURL, capToken string) error {
	client, err := plugins.NewCenterClient(context.Background(), capName, capURL, capToken)
	if err != nil {
		return err
	}
	return client.SyncCapabilityFromCenter()
}

// AddCapabilityIntoCluster will add a capability into K8s cluster, it is equal to apply a definition yaml and run `vela workloads/traits`
func AddCapabilityIntoCluster(c client.Client, mapper discoverymapper.DiscoveryMapper, capability string) (string, error) {
	ss := strings.Split(capability, "/")
	if len(ss) < 2 {
		return "", errors.New("invalid format for " + capability + ", please follow format <center>/<name>")
	}
	repoName := ss[0]
	name := ss[1]
	ioStreams := cmdutil.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr}
	if err := InstallCapability(c, mapper, repoName, name, ioStreams); err != nil {
		return "", err
	}
	return fmt.Sprintf("Successfully installed capability %s from %s", name, repoName), nil
}

// InstallCapability will add a cap into K8s cluster and install it's controller(helm charts)
func InstallCapability(client client.Client, mapper discoverymapper.DiscoveryMapper, centerName, capabilityName string, ioStreams cmdutil.IOStreams) error {
	dir, _ := system.GetCapCenterDir()
	repoDir := filepath.Join(dir, centerName)
	tp, err := GetCapabilityFromCenter(centerName, capabilityName)
	if err != nil {
		return err
	}
	tp.Source = &types.Source{RepoName: centerName}
	defDir, _ := system.GetCapabilityDir()
	switch tp.Type {
	case types.TypeWorkload:
		var wd v1alpha2.WorkloadDefinition
		workloadData, err := ioutil.ReadFile(filepath.Clean(filepath.Join(repoDir, tp.Name+".yaml")))
		if err != nil {
			return err
		}
		if err = yaml.Unmarshal(workloadData, &wd); err != nil {
			return err
		}
		wd.Namespace = types.DefaultKubeVelaNS
		ioStreams.Info("Installing workload capability " + wd.Name)
		if tp.Install != nil {
			tp.Source.ChartName = tp.Install.Helm.Name
			if err = helm.InstallHelmChart(ioStreams, tp.Install.Helm); err != nil {
				return err
			}
		}
		gvk, err := util.GetGVKFromDefinition(mapper, wd.Spec.Reference)
		if err != nil {
			return err
		}
		tp.CrdInfo = &types.CRDInfo{
			APIVersion: gvk.GroupVersion().String(),
			Kind:       gvk.Kind,
		}
		err = addSourceIntoExtension(wd.Spec.Extension, tp.Source)
		if err != nil {
			return err
		}
		if err = client.Create(context.Background(), &wd); err != nil && !apierrors.IsAlreadyExists(err) {
			return err
		}
	case types.TypeTrait:
		var td v1alpha2.TraitDefinition
		traitdata, err := ioutil.ReadFile(filepath.Clean(filepath.Join(repoDir, tp.Name+".yaml")))
		if err != nil {
			return err
		}
		if err = yaml.Unmarshal(traitdata, &td); err != nil {
			return err
		}
		td.Namespace = types.DefaultKubeVelaNS
		ioStreams.Info("Installing trait capability " + td.Name)
		if tp.Install != nil {
			tp.Source.ChartName = tp.Install.Helm.Name
			if err = helm.InstallHelmChart(ioStreams, tp.Install.Helm); err != nil {
				return err
			}
		}
		if err = HackForStandardTrait(tp, client); err != nil {
			return err
		}
		gvk, err := util.GetGVKFromDefinition(mapper, td.Spec.Reference)
		if err != nil {
			return err
		}
		tp.CrdInfo = &types.CRDInfo{
			APIVersion: gvk.GroupVersion().String(),
			Kind:       gvk.Kind,
		}
		err = addSourceIntoExtension(td.Spec.Extension, tp.Source)
		if err != nil {
			return err
		}
		if err = client.Create(context.Background(), &td); err != nil && !apierrors.IsAlreadyExists(err) {
			return err
		}
	case types.TypeScope:
		// TODO(wonderflow): support install scope here
	case types.TypeComponentDefinition:
		// TODO(yangsoon): support ComponentDefinition here
	}

	success := plugins.SinkTemp2Local([]types.Capability{tp}, defDir)
	if success == 1 {
		ioStreams.Infof("Successfully installed capability %s from %s\n", capabilityName, centerName)
	}
	return nil
}

// HackForStandardTrait will do some hack install for standard capability
func HackForStandardTrait(tp types.Capability, client client.Client) error {
	switch tp.Name {
	case "metrics":
		// metrics trait will rely on a Prometheus instance to be installed
		// make sure the chart is a prometheus operator
		if tp.Install == nil {
			break
		}
		if tp.Install.Helm.Namespace == "monitoring" && tp.Install.Helm.Name == "kube-prometheus-stack" {
			if err := InstallPrometheusInstance(client); err != nil {
				return err
			}
		}
	default:
	}
	return nil
}

// GetCapabilityFromCenter will list all synced capabilities from cap center and return the specified one
func GetCapabilityFromCenter(repoName, addonName string) (types.Capability, error) {
	dir, _ := system.GetCapCenterDir()
	repoDir := filepath.Join(dir, repoName)
	templates, err := plugins.LoadCapabilityFromSyncedCenter(repoDir)
	if err != nil {
		return types.Capability{}, err
	}
	for _, t := range templates {
		if t.Name == addonName {
			return t, nil
		}
	}
	return types.Capability{}, fmt.Errorf("%s/%s not exist, try 'vela cap center sync %s' to sync from remote", repoName, addonName, repoName)
}

// ListCapabilityCenters will list all capabilities from center
func ListCapabilityCenters() ([]apis.CapabilityCenterMeta, error) {
	var capabilityCenterList []apis.CapabilityCenterMeta
	centers, err := plugins.LoadRepos()
	if err != nil {
		return capabilityCenterList, err
	}
	for _, c := range centers {
		capabilityCenterList = append(capabilityCenterList, apis.CapabilityCenterMeta{
			Name: c.Name,
			URL:  c.Address,
		})
	}
	return capabilityCenterList, nil
}

// SyncCapabilityCenter will sync capabilities from center to local
func SyncCapabilityCenter(capabilityCenterName string) error {
	repos, err := plugins.LoadRepos()
	if err != nil {
		return err
	}
	if len(repos) == 0 {
		return fmt.Errorf("no capability center configured")
	}
	find := false
	if capabilityCenterName != "" {
		for idx, r := range repos {
			if r.Name == capabilityCenterName {
				repos = []plugins.CapCenterConfig{repos[idx]}
				find = true
				break
			}
		}
		if !find {
			return fmt.Errorf("%s center not exist", capabilityCenterName)
		}
	}
	ctx := context.Background()
	for _, d := range repos {
		client, err := plugins.NewCenterClient(ctx, d.Name, d.Address, d.Token)
		if err != nil {
			return err
		}
		err = client.SyncCapabilityFromCenter()
		if err != nil {
			return err
		}
	}
	return nil
}

// RemoveCapabilityFromCluster will remove a capability from cluster.
// 1. remove definition 2. uninstall chart 3. remove local files
func RemoveCapabilityFromCluster(userNamespace string, c common.Args, client client.Client, capabilityName string) (string, error) {
	ioStreams := cmdutil.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr}
	if err := RemoveCapability(userNamespace, c, client, capabilityName, ioStreams); err != nil {
		return "", err
	}
	msg := fmt.Sprintf("%s removed successfully", capabilityName)
	return msg, nil
}

// RemoveCapability will remove a capability from cluster.
// 1. remove definition 2. uninstall chart 3. remove local files
func RemoveCapability(userNamespace string, c common.Args, client client.Client, capabilityName string, ioStreams cmdutil.IOStreams) error {
	// TODO(wonderflow): make sure no apps is using this capability
	caps, err := plugins.LoadAllInstalledCapability(userNamespace, c)
	if err != nil {
		return err
	}
	for _, w := range caps {
		if w.Name == capabilityName {
			return uninstallCap(client, w, ioStreams)
		}
	}
	return errors.New(capabilityName + " not exist")
}

func uninstallCap(client client.Client, cap types.Capability, ioStreams cmdutil.IOStreams) error {
	// 1. Remove WorkloadDefinition or TraitDefinition
	ctx := context.Background()
	var obj runtime.Object
	switch cap.Type {
	case types.TypeTrait:
		obj = &v1alpha2.TraitDefinition{ObjectMeta: v1.ObjectMeta{Name: cap.Name, Namespace: types.DefaultKubeVelaNS}}
	case types.TypeWorkload:
		obj = &v1alpha2.WorkloadDefinition{ObjectMeta: v1.ObjectMeta{Name: cap.Name, Namespace: types.DefaultKubeVelaNS}}
	case types.TypeScope:
		return fmt.Errorf("uninstall scope capability was not supported yet")
	case types.TypeComponentDefinition:
		// TODO(yangsoon): support uninstall componentDefinition here
	}
	if err := client.Delete(ctx, obj); err != nil {
		return err
	}

	if cap.Install != nil && cap.Install.Helm.Name != "" {
		// 2. Remove Helm chart if there is
		if cap.Install.Helm.Namespace == "" {
			cap.Install.Helm.Namespace = types.DefaultKubeVelaNS
		}
		if err := helm.Uninstall(ioStreams, cap.Install.Helm.Name, cap.Install.Helm.Namespace, cap.Name); err != nil {
			return err
		}
	}

	// 3. Remove local capability file
	capdir, _ := system.GetCapabilityDir()
	switch cap.Type {
	case types.TypeTrait:
		if err := os.Remove(filepath.Join(capdir, "traits", cap.Name)); err != nil {
			return err
		}
	case types.TypeWorkload:
		if err := os.Remove(filepath.Join(capdir, "workloads", cap.Name)); err != nil {
			return err
		}
	case types.TypeScope:
		// TODO(wonderflow): add scope remove here.
	case types.TypeComponentDefinition:
		// TODO(yangsoon): add componenetDefinition remove here
	}
	ioStreams.Infof("Successfully uninstalled capability %s", cap.Name)
	return nil
}

// ListCapabilities will list all caps from specified center
func ListCapabilities(userNamespace string, c common.Args, capabilityCenterName string) ([]types.Capability, error) {
	var capabilityList []types.Capability
	dir, err := system.GetCapCenterDir()
	if err != nil {
		return capabilityList, err
	}
	if capabilityCenterName != "" {
		return listCenterCapabilities(userNamespace, c, filepath.Join(dir, capabilityCenterName))
	}
	dirs, err := ioutil.ReadDir(dir)
	if err != nil {
		return capabilityList, err
	}
	for _, dd := range dirs {
		if !dd.IsDir() {
			continue
		}
		caps, err := listCenterCapabilities(userNamespace, c, filepath.Join(dir, dd.Name()))
		if err != nil {
			return capabilityList, err
		}
		capabilityList = append(capabilityList, caps...)
	}
	return capabilityList, nil
}

func listCenterCapabilities(userNamespace string, c common.Args, repoDir string) ([]types.Capability, error) {
	templates, err := plugins.LoadCapabilityFromSyncedCenter(repoDir)
	if err != nil {
		return templates, err
	}
	if len(templates) < 1 {
		return templates, nil
	}
	baseDir := filepath.Base(repoDir)
	workloads := gatherWorkloads(userNamespace, c, templates)
	for i, p := range templates {
		status := checkInstallStatus(userNamespace, c, baseDir, p)
		convertedApplyTo := ConvertApplyTo(p.AppliesTo, workloads)
		templates[i].Center = baseDir
		templates[i].Status = status
		templates[i].AppliesTo = convertedApplyTo
	}
	return templates, nil
}

// RemoveCapabilityCenter will remove a cap center from local
func RemoveCapabilityCenter(centerName string) (string, error) {
	var message string
	var err error
	dir, _ := system.GetCapCenterDir()
	repoDir := filepath.Join(dir, centerName)
	// 1.remove capability center dir
	if _, err := os.Stat(repoDir); err != nil {
		if os.IsNotExist(err) {
			err = fmt.Errorf("%s capability center has not successfully synced", centerName)
			return message, err
		}
	}
	if err = os.RemoveAll(repoDir); err != nil {
		return message, err
	}
	// 2.remove center from capability center config
	repos, err := plugins.LoadRepos()
	if err != nil {
		return message, err
	}
	for idx, r := range repos {
		if r.Name == centerName {
			repos = append(repos[:idx], repos[idx+1:]...)
			break
		}
	}
	if err = plugins.StoreRepos(repos); err != nil {
		return message, err
	}
	message = fmt.Sprintf("%s capability center removed successfully", centerName)
	return message, err
}

func gatherWorkloads(userNamespace string, c common.Args, templates []types.Capability) []types.Capability {
	workloads, err := plugins.LoadInstalledCapabilityWithType(userNamespace, c, types.TypeWorkload)
	if err != nil {
		workloads = make([]types.Capability, 0)
	}
	for _, t := range templates {
		if t.Type == types.TypeWorkload {
			workloads = append(workloads, t)
		}
	}
	return workloads
}

func checkInstallStatus(userNamespace string, c common.Args, repoName string, tmp types.Capability) string {
	var status = "uninstalled"
	installed, _ := plugins.LoadInstalledCapabilityWithType(userNamespace, c, tmp.Type)
	for _, i := range installed {
		if i.Source != nil && i.Source.RepoName == repoName && i.Name == tmp.Name && i.CrdName == tmp.CrdName {
			return "installed"
		}
	}
	return status
}

func addSourceIntoExtension(in *runtime.RawExtension, source *types.Source) error {
	var extension map[string]interface{}
	err := json.Unmarshal(in.Raw, &extension)
	if err != nil {
		return err
	}
	extension["source"] = source
	data, err := json.Marshal(extension)
	if err != nil {
		return err
	}
	in.Raw = data
	return nil
}

// GetCapabilityConfigMap gets the ConfigMap which stores the information of a capability
func GetCapabilityConfigMap(kubeClient client.Client, capabilityName string) (corev1.ConfigMap, error) {
	cmName := fmt.Sprintf("%s%s", types.CapabilityConfigMapNamePrefix, capabilityName)
	var cm corev1.ConfigMap
	err := kubeClient.Get(context.Background(), client.ObjectKey{Namespace: types.DefaultKubeVelaNS, Name: cmName}, &cm)
	return cm, err
}

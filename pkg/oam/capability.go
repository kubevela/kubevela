package oam

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
	"github.com/ghodss/yaml"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/api/types"
	cmdutil "github.com/oam-dev/kubevela/pkg/commands/util"
	"github.com/oam-dev/kubevela/pkg/plugins"
	"github.com/oam-dev/kubevela/pkg/server/apis"
	"github.com/oam-dev/kubevela/pkg/utils/helm"
	"github.com/oam-dev/kubevela/pkg/utils/system"
)

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

func SyncCapabilityFromCenter(capName, capURL, capToken string) error {
	client, err := plugins.NewCenterClient(context.Background(), capName, capURL, capToken)
	if err != nil {
		return err
	}
	return client.SyncCapabilityFromCenter()
}

func AddCapabilityIntoCluster(c client.Client, capability string) (string, error) {
	ss := strings.Split(capability, "/")
	if len(ss) < 2 {
		return "", errors.New("invalid format for " + capability + ", please follow format <center>/<name>")
	}
	repoName := ss[0]
	name := ss[1]
	ioStreams := cmdutil.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr}
	if err := InstallCapability(c, repoName, name, ioStreams); err != nil {
		return "", err
	}
	return fmt.Sprintf("Successfully installed capability %s from %s", name, repoName), nil
}

func InstallCapability(client client.Client, centerName, capabilityName string, ioStreams cmdutil.IOStreams) error {
	dir, _ := system.GetCapCenterDir()
	repoDir := filepath.Join(dir, centerName)
	tp, err := GetSyncedCapabilities(centerName, capabilityName)
	if err != nil {
		return err
	}
	tp.Source = &types.Source{RepoName: centerName}
	defDir, _ := system.GetCapabilityDir()
	switch tp.Type {
	case types.TypeWorkload:
		var wd v1alpha2.WorkloadDefinition
		workloadData, err := ioutil.ReadFile(filepath.Join(repoDir, tp.CrdName+".yaml"))
		if err != nil {
			return nil
		}
		if err = yaml.Unmarshal(workloadData, &wd); err != nil {
			return err
		}
		wd.Namespace = types.DefaultOAMNS
		ioStreams.Info("Installing workload capability " + wd.Name)
		if tp.Install != nil {
			tp.Source.ChartName = tp.Install.Helm.Name
			if err = InstallHelmChart(ioStreams, tp.Install.Helm); err != nil {
				return err
			}
		}
		if apiVerion, kind := cmdutil.GetAPIVersionKindFromWorkload(wd); apiVerion != "" && kind != "" {
			tp.CrdInfo = &types.CrdInfo{
				APIVersion: apiVerion,
				Kind:       kind,
			}
		}
		if err = client.Create(context.Background(), &wd); err != nil && !apierrors.IsAlreadyExists(err) {
			return err
		}
	case types.TypeTrait:
		var td v1alpha2.TraitDefinition
		traitdata, err := ioutil.ReadFile(filepath.Join(repoDir, tp.CrdName+".yaml"))
		if err != nil {
			return nil
		}
		if err = yaml.Unmarshal(traitdata, &td); err != nil {
			return err
		}
		td.Namespace = types.DefaultOAMNS
		ioStreams.Info("Installing trait capability " + td.Name)
		if tp.Install != nil {
			tp.Source.ChartName = tp.Install.Helm.Name
			if err = InstallHelmChart(ioStreams, tp.Install.Helm); err != nil {
				return err
			}
		}
		if apiVerion, kind := cmdutil.GetAPIVersionKindFromTrait(td); apiVerion != "" && kind != "" {
			tp.CrdInfo = &types.CrdInfo{
				APIVersion: apiVerion,
				Kind:       kind,
			}
		}
		if err = client.Create(context.Background(), &td); err != nil && !apierrors.IsAlreadyExists(err) {
			return err
		}
	case types.TypeScope:
		//TODO(wonderflow): support install scope here
	}

	success := plugins.SinkTemp2Local([]types.Capability{tp}, defDir)
	if success == 1 {
		ioStreams.Infof("Successfully installed capability %s from %s\n", capabilityName, centerName)
	}
	return nil
}

func GetSyncedCapabilities(repoName, addonName string) (types.Capability, error) {
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
	return types.Capability{}, fmt.Errorf("%s/%s not exist, try vela cap:center:sync %s to sync from remote", repoName, addonName, repoName)
}

func InstallHelmChart(ioStreams cmdutil.IOStreams, c types.Chart) error {
	return helm.Install(ioStreams, c.Repo, c.URL, c.Name, c.Version, c.Namespace, c.Name, nil)
}

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

func RemoveCapabilityFromCluster(client client.Client, capabilityName string) (string, error) {
	ioStreams := cmdutil.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr}
	if err := RemoveCapability(client, capabilityName, ioStreams); err != nil {
		return "", err
	}
	msg := fmt.Sprintf("%s removed successfully", capabilityName)
	return msg, nil
}

func RemoveCapability(client client.Client, capabilityName string, ioStreams cmdutil.IOStreams) error {
	// TODO(wonderflow): make sure no apps is using this capability
	caps, err := plugins.LoadAllInstalledCapability()
	if err != nil {
		return err
	}
	for _, w := range caps {
		if w.Name == capabilityName {
			return UninstallCap(client, w, ioStreams)
		}
	}
	return errors.New(capabilityName + " not exist")
}

func UninstallCap(client client.Client, cap types.Capability, ioStreams cmdutil.IOStreams) error {
	// 1. Remove WorkloadDefinition or TraitDefinition
	ctx := context.Background()
	var obj runtime.Object
	switch cap.Type {
	case types.TypeTrait:
		obj = &v1alpha2.TraitDefinition{ObjectMeta: v1.ObjectMeta{Name: cap.CrdName, Namespace: types.DefaultOAMNS}}
	case types.TypeWorkload:
		obj = &v1alpha2.WorkloadDefinition{ObjectMeta: v1.ObjectMeta{Name: cap.CrdName, Namespace: types.DefaultOAMNS}}
	}
	if err := client.Delete(ctx, obj); err != nil {
		return err
	}

	if cap.Install != nil && cap.Install.Helm.Name != "" {
		// 2. Remove Helm chart if there is
		if err := helm.Uninstall(ioStreams, cap.Install.Helm.Name, types.DefaultOAMNS, cap.Name); err != nil {
			return err
		}
	}

	// 3. Remove local capability file
	capdir, _ := system.GetCapabilityDir()
	switch cap.Type {
	case types.TypeTrait:
		return os.Remove(filepath.Join(capdir, "traits", cap.Name))
	case types.TypeWorkload:
		return os.Remove(filepath.Join(capdir, "workloads", cap.Name))
	}
	ioStreams.Infof("%s removed successfully", cap.Name)
	return nil
}

func ListCapabilities(capabilityCenterName string) ([]types.Capability, error) {
	var capabilityList []types.Capability
	dir, err := system.GetCapCenterDir()
	if err != nil {
		return capabilityList, err
	}
	if capabilityCenterName != "" {
		return ListCenterCapabilities(filepath.Join(dir, capabilityCenterName))
	}
	dirs, err := ioutil.ReadDir(dir)
	if err != nil {
		return capabilityList, err
	}
	for _, dd := range dirs {
		if !dd.IsDir() {
			continue
		}
		caps, err := ListCenterCapabilities(filepath.Join(dir, dd.Name()))
		if err != nil {
			return capabilityList, err
		}
		capabilityList = append(capabilityList, caps...)
	}
	return capabilityList, nil
}

func ListCenterCapabilities(repoDir string) ([]types.Capability, error) {
	templates, err := plugins.LoadCapabilityFromSyncedCenter(repoDir)
	if err != nil {
		return templates, err
	}
	if len(templates) < 1 {
		return templates, nil
	}
	baseDir := filepath.Base(repoDir)
	workloads := GatherWorkloads(templates)
	for i, p := range templates {
		status := CheckInstallStatus(baseDir, p)
		convertedApplyTo := ConvertApplyTo(p.AppliesTo, workloads)
		templates[i].Center = baseDir
		templates[i].Status = status
		templates[i].AppliesTo = convertedApplyTo
	}
	return templates, nil
}

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

func GatherWorkloads(templates []types.Capability) []types.Capability {
	workloads, err := plugins.LoadInstalledCapabilityWithType(types.TypeWorkload)
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

func CheckInstallStatus(repoName string, tmp types.Capability) string {
	var status = "uninstalled"
	installed, _ := plugins.LoadInstalledCapabilityWithType(tmp.Type)
	for _, i := range installed {
		if i.Source != nil && i.Source.RepoName == repoName && i.Name == tmp.Name && i.CrdName == tmp.CrdName {
			return "installed"
		}
	}
	return status
}

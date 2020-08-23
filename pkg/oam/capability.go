package oam

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strings"

	"github.com/cloud-native-application/rudrx/pkg/server/apis"

	cmdutil "github.com/cloud-native-application/rudrx/pkg/cmd/util"

	"github.com/cloud-native-application/rudrx/api/types"
	"github.com/cloud-native-application/rudrx/pkg/plugins"
	"github.com/cloud-native-application/rudrx/pkg/utils/system"
	"github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
	"gopkg.in/yaml.v2"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func AddCapabilityCenter(capName, capUrl, capToken string) error {
	repos, err := plugins.LoadRepos()
	if err != nil {
		return err
	}
	config := &plugins.CapCenterConfig{
		Name:    capName,
		Address: capUrl,
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
	return SyncCapabilityFromCenter(capName, capUrl, capToken)
}

func SyncCapabilityFromCenter(capName, capUrl, capToken string) error {
	client, err := plugins.NewCenterClient(context.Background(), capName, capUrl, capToken)
	if err != nil {
		return err
	}
	return client.SyncCapabilityFromCenter()
}

func AddCapabilityIntoCluster(capability string, c client.Client) (string, error) {
	ss := strings.Split(capability, "/")
	if len(ss) < 2 {
		return "", errors.New("invalid format for " + capability + ", please follow format <center>/<name>")
	}
	repoName := ss[0]
	name := ss[1]
	var ioStreams cmdutil.IOStreams
	if err := InstallCapability(c, repoName, name, ioStreams); err != nil {
		return "", err
	}
	return fmt.Sprintf("Successfully installed capability %s from %s\n", repoName, name), nil
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
		if apiVerion, kind := cmdutil.GetApiVersionKindFromWorkload(wd); apiVerion != "" && kind != "" {
			tp.CrdInfo = &types.CrdInfo{
				ApiVersion: apiVerion,
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
		if apiVerion, kind := cmdutil.GetApiVersionKindFromTrait(td); apiVerion != "" && kind != "" {
			tp.CrdInfo = &types.CrdInfo{
				ApiVersion: apiVerion,
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
	return HelmInstall(ioStreams, c.Repo, c.URl, c.Name, c.Version, c.Name, nil)
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
			Url:  c.Address,
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
		err = client.SyncCapabilityFromCenter()
		if err != nil {
			return err
		}
	}
	return nil
}

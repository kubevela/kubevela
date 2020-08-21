package cmd

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/runtime"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/ghodss/yaml"

	"github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"

	"github.com/gosuri/uitable"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/cloud-native-application/rudrx/pkg/oam"
	"github.com/cloud-native-application/rudrx/pkg/utils/system"

	"github.com/cloud-native-application/rudrx/pkg/plugins"

	"github.com/cloud-native-application/rudrx/api/types"

	cmdutil "github.com/cloud-native-application/rudrx/pkg/cmd/util"
	"github.com/spf13/cobra"
)

func CapabilityCommandGroup(parentCmd *cobra.Command, c types.Args, ioStream cmdutil.IOStreams) {
	parentCmd.AddCommand(
		NewCapCenterConfigCommand(ioStream),
		NewCapListCommand(ioStream),
		NewCapCenterSyncCommand(ioStream),
		NewCapAddCommand(c, ioStream),
		NewCapRemoveCommand(c, ioStream),
		NewCapCenterListCommand(ioStream),
	)
}

func NewCapCenterConfigCommand(ioStreams cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "cap:center:config <centerName> <centerUrl>",
		Short:   "Configure or add the capability center, default is local (built-in capabilities)",
		Long:    "Configure or add the capability center, default is local (built-in capabilities)",
		Example: `vela cap:center:config mycenter https://github.com/oam-dev/catalog/cap-center`,
		RunE: func(cmd *cobra.Command, args []string) error {
			argsLength := len(args)
			if argsLength < 2 {
				return errors.New("please set capability center with <centerName> and <centerUrl>")
			}
			repos, err := plugins.LoadRepos()
			if err != nil {
				return err
			}
			config := &plugins.CapCenterConfig{
				Name:    args[0],
				Address: args[1],
				Token:   cmd.Flag("token").Value.String(),
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
			ioStreams.Info(fmt.Sprintf("Successfully configured capability center: %s, start to sync from remote", args[0]))
			client, err := plugins.NewCenterClient(context.Background(), config.Name, config.Address, config.Token)
			err = client.SyncCapabilityFromCenter()
			if err != nil {
				return err
			}
			ioStreams.Info("sync finished")
			return nil
		},
		Annotations: map[string]string{
			types.TagCommandType: types.TypeOthers,
		},
	}
	cmd.PersistentFlags().StringP("token", "t", "", "Github Repo token")
	return cmd
}

func NewCapAddCommand(c types.Args, ioStreams cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "cap:add <center>/<name>",
		Short:   "Add capability into cluster",
		Long:    "Add capability into cluster",
		Example: `vela cap:add mycenter/route`,
		RunE: func(cmd *cobra.Command, args []string) error {
			argsLength := len(args)
			if argsLength < 1 {
				return errors.New("you must specify <center>/<name> for capability you want to add")
			}
			newClient, err := client.New(c.Config, client.Options{Scheme: c.Schema})
			if err != nil {
				return err
			}
			ss := strings.Split(args[0], "/")
			if len(ss) < 2 {
				return errors.New("invalid format for " + args[0] + ", please follow format <center>/<name>")
			}
			repoName := ss[0]
			name := ss[1]
			return InstallCapability(newClient, repoName, name, ioStreams)
		},
		Annotations: map[string]string{
			types.TagCommandType: types.TypeOthers,
		},
	}
	cmd.PersistentFlags().StringP("token", "t", "", "Github Repo token")
	return cmd
}

func NewCapRemoveCommand(c types.Args, ioStreams cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "cap:remove <name>",
		Short:   "Remove capability from cluster",
		Long:    "Remove capability from cluster",
		Example: `vela cap:remove route`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) < 1 {
				return errors.New("you must specify <name> for capability you want to remove")
			}
			newClient, err := client.New(c.Config, client.Options{Scheme: c.Schema})
			if err != nil {
				return err
			}
			name := args[0]
			if strings.Contains(name, "/") {
				l := strings.Split(name, "/")
				if len(l) > 2 {
					return fmt.Errorf("invalid format '%s', you can't contain more than one / in name", name)
				}
				name = l[1]
			}
			return RemoveCapability(newClient, name, ioStreams)
		},
		Annotations: map[string]string{
			types.TagCommandType: types.TypeOthers,
		},
	}
	cmd.PersistentFlags().StringP("token", "t", "", "Github Repo token")
	return cmd
}

func NewCapCenterSyncCommand(ioStreams cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "cap:center:sync [centerName]",
		Short:   "Sync capabilities from remote center, default to sync all centers",
		Long:    "Sync capabilities from remote center, default to sync all centers",
		Example: `vela cap:center:sync mycenter`,
		RunE: func(cmd *cobra.Command, args []string) error {
			repos, err := plugins.LoadRepos()
			if err != nil {
				return err
			}
			var specified string
			if len(args) > 0 {
				specified = args[0]
			}
			if len(repos) == 0 {
				return fmt.Errorf("no capability center configured")
			}
			find := false
			if specified != "" {
				for idx, r := range repos {
					if r.Name == specified {
						repos = []plugins.CapCenterConfig{repos[idx]}
						find = true
						break
					}
				}
				if !find {
					return fmt.Errorf("%s center not exist", specified)
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
			ioStreams.Info("sync finished")
			return nil
		},
		Annotations: map[string]string{
			types.TagCommandType: types.TypeOthers,
		},
	}
	return cmd
}

func NewCapListCommand(ioStreams cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "cap:ls [centerName]",
		Short:   "List all capabilities in center",
		Long:    "List all capabilities in center",
		Example: `vela cap:ls`,
		RunE: func(cmd *cobra.Command, args []string) error {
			var repoName string
			if len(args) > 0 {
				repoName = args[0]
			}
			dir, err := system.GetCapCenterDir()
			if err != nil {
				return err
			}
			table := uitable.New()
			table.AddRow("NAME", "CENTER", "TYPE", "DEFINITION", "STATUS", "APPLIES-TO")
			if repoName != "" {
				if err = ListCenterCapabilities(table, filepath.Join(dir, repoName), ioStreams); err != nil {
					return err
				}
				ioStreams.Info(table.String())
				return nil
			}
			dirs, err := ioutil.ReadDir(dir)
			if err != nil {
				return err
			}
			for _, dd := range dirs {
				if !dd.IsDir() {
					continue
				}
				if err = ListCenterCapabilities(table, filepath.Join(dir, dd.Name()), ioStreams); err != nil {
					return err
				}
			}
			ioStreams.Info(table.String())
			return nil
		},
		Annotations: map[string]string{
			types.TagCommandType: types.TypeOthers,
		},
	}
	return cmd
}

func NewCapCenterListCommand(ioStreams cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "cap:center:ls",
		Short:   "List all capability centers",
		Long:    "List all configured capability centers",
		Example: `vela cap:center:ls`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return ListCapCenters(args, ioStreams)
		},
		Annotations: map[string]string{
			types.TagCommandType: types.TypeOthers,
		},
	}
	return cmd
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
		if err := HelmUninstall(ioStreams, cap.Install.Helm.Name, cap.Name); err != nil {
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

func InstallHelmChart(ioStreams cmdutil.IOStreams, c types.Chart) error {
	return HelmInstall(ioStreams, c.Repo, c.URl, c.Name, c.Version, c.Name, nil)
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

func ListCenterCapabilities(table *uitable.Table, repoDir string, ioStreams cmdutil.IOStreams) error {
	templates, err := plugins.LoadCapabilityFromSyncedCenter(repoDir)
	if err != nil {
		return err
	}
	if len(templates) < 1 {
		return nil
	}
	baseDir := filepath.Base(repoDir)
	workloads := GatherWorkloads(templates)
	for _, p := range templates {
		status := CheckInstallStatus(baseDir, p)
		convertedApplyTo := oam.ConvertApplyTo(p.AppliesTo, workloads)
		table.AddRow(p.Name, baseDir, p.Type, p.CrdName, status, convertedApplyTo)
	}
	return nil
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

func ListCapCenters(args []string, ioStreams cmdutil.IOStreams) error {
	table := uitable.New()
	table.AddRow("NAME", "ADDRESS")
	centers, err := plugins.LoadRepos()
	if err != nil {
		return err
	}
	for _, c := range centers {
		table.AddRow(c.Name, c.Address)
	}
	ioStreams.Info(table.String())
	return nil
}

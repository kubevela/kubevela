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

func CapabilityCommandGroup(c types.Args, ioStream cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cap <command>",
		Short: "Capability Management",
		Long:  "Capability Management with config, list, add, remove capabilities",
		Annotations: map[string]string{
			types.TagCommandType: types.TypeOthers,
		},
	}
	cmd.AddCommand(
		NewCenterCommand(c, ioStream),
		NewCapListCommand(ioStream),
		NewCapAddCommand(c, ioStream),
		NewCapRemoveCommand(c, ioStream),
	)
	return cmd
}

func NewCenterCommand(c types.Args, ioStream cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "center <command>",
		Short: "Manage Capability Center",
		Long:  "Manage Capability Center with config, sync, list",
		Annotations: map[string]string{
			types.TagCommandType: types.TypeOthers,
		},
	}
	cmd.AddCommand(
		NewCapCenterConfigCommand(ioStream),
		NewCapCenterSyncCommand(ioStream),
		NewCapCenterListCommand(ioStream),
	)
	return cmd
}

func NewCapCenterConfigCommand(ioStreams cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "config <centerName> <centerUrl>",
		Short:   "Configure or add the capability center, default is local (built-in capabilities)",
		Long:    "Configure or add the capability center, default is local (built-in capabilities)",
		Example: `vela cap center config mycenter https://github.com/oam-dev/catalog/cap-center`,
		RunE: func(cmd *cobra.Command, args []string) error {
			argsLength := len(args)
			if argsLength < 2 {
				return errors.New("please set capability center with <centerName> and <centerUrl>")
			}
			capName := args[0]
			capUrl := args[1]
			token := cmd.Flag("token").Value.String()
			if err := oam.AddCapabilityCenter(capName, capUrl, token); err != nil {
				return err
			}
			ioStreams.Infof("Successfully configured capability center: %s, start to sync from remote", capName)
			if err := oam.SyncCapabilityFromCenter(capName, capUrl, token); err != nil {
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
		Use:     "add <center>/<name>",
		Short:   "Add capability into cluster",
		Long:    "Add capability into cluster",
		Example: `vela cap add mycenter/route`,
		RunE: func(cmd *cobra.Command, args []string) error {
			var err error
			argsLength := len(args)
			if argsLength < 1 {
				return errors.New("you must specify <center>/<name> for capability you want to add")
			}
			newClient, err := client.New(c.Config, client.Options{Scheme: c.Schema})
			if err != nil {
				return err
			}
			var msg string
			if msg, err = oam.AddCapabilityIntoCluster(args[0], newClient); err != nil {
				return err
			}
			ioStreams.Info(msg)
			return nil
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
		Use:     "remove <name>",
		Short:   "Remove capability from cluster",
		Long:    "Remove capability from cluster",
		Example: `vela cap remove route`,
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
		Use:     "sync [centerName]",
		Short:   "Sync capabilities from remote center, default to sync all centers",
		Long:    "Sync capabilities from remote center, default to sync all centers",
		Example: `vela cap center sync mycenter`,
		RunE: func(cmd *cobra.Command, args []string) error {
			var specified string
			if len(args) > 0 {
				specified = args[0]
			}
			if err := oam.SyncCapabilityCenter(specified); err != nil {
				return err
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
		Use:     "ls [centerName]",
		Short:   "List all capabilities in center",
		Long:    "List all capabilities in center",
		Example: `vela cap ls`,
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
		Use:     "ls",
		Short:   "List all capability centers",
		Long:    "List all configured capability centers",
		Example: `vela cap center ls`,
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
		if err := oam.HelmUninstall(ioStreams, cap.Install.Helm.Name, cap.Name); err != nil {
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
	capabilityCenterList, err := oam.ListCapabilityCenters()
	if err != nil {
		return err
	}
	for _, c := range capabilityCenterList {
		table.AddRow(c.Name, c.Url)
	}
	ioStreams.Info(table.String())
	return nil
}

package cmd

import (
	"path/filepath"
	"strings"

	"github.com/cloud-native-application/rudrx/api/types"

	"github.com/cloud-native-application/rudrx/pkg/plugins"
	"github.com/cloud-native-application/rudrx/pkg/utils/system"

	cmdutil "github.com/cloud-native-application/rudrx/pkg/cmd/util"
	"github.com/gosuri/uitable"
	"github.com/spf13/cobra"
)

func NewTraitsCommand(ioStreams cmdutil.IOStreams) *cobra.Command {
	var workloadName string
	cmd := &cobra.Command{
		Use:                   "traits [--apply-to WORKLOADNAME]",
		DisableFlagsInUseLine: true,
		Short:                 "List traits",
		Long:                  "List traits",
		Example:               `vela traits`,
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, _ := system.GetCapabilityDir()
			templates, err := plugins.LoadTempFromLocal(filepath.Join(dir, "traits"))
			if err != nil {
				return err
			}
			workloads, err := plugins.LoadTempFromLocal(filepath.Join(dir, "workloads"))
			if err != nil {
				return err
			}
			return printTraitList(templates, workloads, &workloadName, ioStreams)
		},
	}

	cmd.SetOut(ioStreams.Out)
	cmd.Flags().StringVar(&workloadName, "apply-to", "", "Workload name")
	return cmd
}

func printTraitList(traits, workloads []types.Capability, workloadName *string, ioStreams cmdutil.IOStreams) error {
	table := uitable.New()
	table.MaxColWidth = 60

	table.AddRow("NAME", "DEFINITION", "APPLIES TO")
	for _, r := range traits {
		convertedApplyTo := ConvertApplyTo(r.AppliesTo, workloads)
		if *workloadName != "" {
			if !In(convertedApplyTo, *workloadName) {
				continue
			}
			convertedApplyTo = []string{*workloadName}
		}
		if len(convertedApplyTo) > 1 && *workloadName == "" {
			for i, wd := range convertedApplyTo {
				if i > 0 {
					table.AddRow("", "", wd)
				} else {
					table.AddRow(r.Name, r.CrdName, wd)
				}
			}
		} else {
			table.AddRow(r.Name, r.CrdName, strings.Join(convertedApplyTo, ""))
		}
	}
	ioStreams.Info(table.String())

	return nil
}

func ConvertApplyTo(applyTo []string, workloads []types.Capability) []string {
	var converted []string
	for _, v := range applyTo {
		newName, exist := check(v, workloads)
		if !exist {
			continue
		}
		converted = append(converted, newName)
	}
	return converted
}

func check(crdname string, workloads []types.Capability) (string, bool) {
	for _, v := range workloads {
		if crdname == v.CrdName {
			return v.Name, true
		}
	}
	return "", false
}

func In(l []string, v string) bool {
	for _, ll := range l {
		if ll == v {
			return true
		}
	}
	return false
}

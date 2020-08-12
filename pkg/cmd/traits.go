package cmd

import (
	"strings"

	"github.com/cloud-native-application/rudrx/api/types"

	"github.com/cloud-native-application/rudrx/pkg/plugins"

	cmdutil "github.com/cloud-native-application/rudrx/pkg/cmd/util"
	plur "github.com/gertd/go-pluralize"
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
			templates, err := plugins.LoadInstalledCapabilityWithType(types.TypeTrait)
			if err != nil {
				return err
			}
			workloads, err := plugins.LoadInstalledCapabilityWithType(types.TypeWorkload)
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

func parse(applyTo string) string {
	l := strings.Split(applyTo, "/")
	if len(l) != 2 {
		return applyTo
	}
	apigroup, versionKind := l[0], l[1]
	l = strings.Split(versionKind, ".")
	if len(l) != 2 {
		return applyTo
	}
	return plur.NewClient().Plural(strings.ToLower(l[1])) + "." + apigroup
}

func check(applyto string, workloads []types.Capability) (string, bool) {
	for _, v := range workloads {
		if parse(applyto) == v.CrdName {
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

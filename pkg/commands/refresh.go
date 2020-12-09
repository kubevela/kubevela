package commands

import (
	"context"

	"github.com/fatih/color"
	"github.com/gosuri/uitable"

	"github.com/oam-dev/kubevela/apis/types"
	cmdutil "github.com/oam-dev/kubevela/pkg/commands/util"
	"github.com/oam-dev/kubevela/pkg/plugins"
	"github.com/oam-dev/kubevela/pkg/utils/system"
)

type refreshStatus string

const (
	added     refreshStatus = "Added"
	updated   refreshStatus = "Updated"
	unchanged refreshStatus = "Unchanged"
	deleted   refreshStatus = "Deleted"
)

// RefreshDefinitions will sync local capabilities with cluster installed ones
func RefreshDefinitions(ctx context.Context, c types.Args, ioStreams cmdutil.IOStreams, silentOutput bool) error {
	dir, _ := system.GetCapabilityDir()

	oldCaps, err := plugins.LoadAllInstalledCapability()
	if err != nil {
		return err
	}
	var syncedTemplates []types.Capability

	templates, templateErrors, err := plugins.GetWorkloadsFromCluster(ctx, types.DefaultKubeVelaNS, c, dir, nil)
	if err != nil {
		return err
	}
	if len(templateErrors) > 0 {
		for _, e := range templateErrors {
			ioStreams.Infof("WARN: %v, you will unable to use this workload capability", e)
		}
	}
	syncedTemplates = append(syncedTemplates, templates...)
	plugins.SinkTemp2Local(templates, dir)

	templates, templateErrors, err = plugins.GetTraitsFromCluster(ctx, types.DefaultKubeVelaNS, c, dir, nil)
	if err != nil {
		return err
	}
	if len(templateErrors) > 0 {
		for _, e := range templateErrors {
			ioStreams.Infof("WARN: %v, you will unable to use this trait capability", e)
		}
	}
	syncedTemplates = append(syncedTemplates, templates...)
	plugins.SinkTemp2Local(templates, dir)
	plugins.RemoveLegacyTemps(syncedTemplates, dir)

	printRefreshReport(syncedTemplates, oldCaps, ioStreams, silentOutput)
	return nil
}

// silent indicates whether output existing caps if no change occurs. If false, output all existing caps.
func printRefreshReport(newCaps, oldCaps []types.Capability, io cmdutil.IOStreams, silent bool) {
	report := refreshResultReport(newCaps, oldCaps)
	table := newUITable()
	table.MaxColWidth = 80
	table.AddRow("TYPE", "CATEGORY", "DESCRIPTION")

	if len(report[added]) == 0 && len(report[updated]) == 0 && len(report[deleted]) == 0 {
		// no change occurs, just show all existing caps
		// always show workload at first
		for _, cap := range report[unchanged] {
			if cap.Type == types.TypeWorkload {
				table.AddRow(cap.Name, cap.Type, cap.Description)
			}
		}
		for _, cap := range report[unchanged] {
			if cap.Type == types.TypeTrait {
				table.AddRow(cap.Name, cap.Type, cap.Description)
			}
		}
		if !silent {
			io.Infof("Automatically discover capabilities successfully %s(no changes)\n\n", emojiSucceed)
			io.Info(table.String())
		}
		return
	}

	io.Infof("Automatically discover capabilities successfully %sAdd(%s) Update(%s) Delete(%s)\n\n",
		emojiSucceed,
		green.Sprint(len(report[added])),
		yellow.Sprint(len(report[updated])),
		red.Sprint(len(report[deleted])))
	// show added/updated/deleted cpas
	addStsRow(added, report, table)
	addStsRow(updated, report, table)
	addStsRow(deleted, report, table)
	io.Info(table.String())
	io.Info()
}

func addStsRow(sts refreshStatus, report map[refreshStatus][]types.Capability, t *uitable.Table) {
	caps := report[sts]
	if len(caps) == 0 {
		return
	}
	var stsIcon string
	var stsColor *color.Color
	switch sts {
	case added:
		stsIcon = "+"
		stsColor = green
	case updated:
		stsIcon = "*"
		stsColor = yellow
	case deleted:
		stsIcon = "-"
		stsColor = red
	case unchanged:
		// normal color display
	}
	for _, cap := range caps {
		t.AddRow(
			// color.New(color.Bold).Sprint(stsColor.Sprint(stsIcon)),
			stsColor.Sprintf("%s%s", stsIcon, cap.Name),
			stsColor.Sprint(cap.Type),
			stsColor.Sprint(cap.Description))
	}
}

func refreshResultReport(newCaps, oldCaps []types.Capability) map[refreshStatus][]types.Capability {
	report := map[refreshStatus][]types.Capability{
		added:     make([]types.Capability, 0),
		updated:   make([]types.Capability, 0),
		unchanged: make([]types.Capability, 0),
		deleted:   make([]types.Capability, 0),
	}
	for _, newCap := range newCaps {
		found := false
		for _, oldCap := range oldCaps {
			if newCap.Name == oldCap.Name {
				found = true
				break
			}
		}
		if !found {
			report[added] = append(report[added], newCap)
		}
	}
	for _, oldCap := range oldCaps {
		found := false
		for _, newCap := range newCaps {
			if oldCap.Name == newCap.Name {
				found = true
				if types.EqualCapability(oldCap, newCap) {
					report[unchanged] = append(report[unchanged], newCap)
				} else {
					report[updated] = append(report[updated], newCap)
				}
				break
			}
		}
		if !found {
			report[deleted] = append(report[deleted], oldCap)
		}
	}
	return report
}

package ui

import (
	"github.com/rivo/tview"

	"github.com/oam-dev/kubevela/references/cli/status-ui/config"
	"github.com/oam-dev/kubevela/references/cli/status-ui/model"
)

type ClusterInfo struct {
	*tview.Table
	Style *config.Style
}

func NewClusterInfo(style *config.Style) *ClusterInfo {
	c := &ClusterInfo{
		Table: tview.NewTable(),
		Style: style,
	}
	c.init()
	return c
}

func (ci *ClusterInfo) init() {
	var row int
	cluster := model.NewCluster()
	ci.SetCell(row, 0, ci.sectionCell("Cluster"))
	ci.SetCell(row, 1, ci.infoCell(cluster.Name()))
	row++

	k8s := cluster.K8SVersion()
	ci.SetCell(row, 0, ci.sectionCell("K8S Version"))
	ci.SetCell(row, 1, ci.infoCell(k8s))
	row++

	vela := cluster.VelaVersion()
	ci.SetCell(row, 0, ci.sectionCell("Vela Version"))
	ci.SetCell(row, 1, ci.infoCell(vela))
	row++

	ci.SetCell(row, 0, ci.sectionCell("CPU"))
	ci.SetCell(row, 1, ci.infoCell("n/a"))
	row++

	ci.SetCell(row, 0, ci.sectionCell("MEM"))
	ci.SetCell(row, 1, ci.infoCell("n/a"))
	ci.refresh()
}

func (*ClusterInfo) sectionCell(t string) *tview.TableCell {
	c := tview.NewTableCell(t + ":")
	c.SetAlign(tview.AlignLeft)
	return c
}

func (*ClusterInfo) infoCell(t string) *tview.TableCell {
	c := tview.NewTableCell(t)
	return c
}

func (ci *ClusterInfo) refresh() {
}

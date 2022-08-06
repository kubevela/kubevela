package component

import (
	"github.com/oam-dev/kubevela/references/cli/top/config"
	"github.com/oam-dev/kubevela/references/cli/top/model"
	"github.com/rivo/tview"
	"k8s.io/client-go/rest"
)

// InfoBoard a component which display system info
type InfoBoard struct {
	*tview.Table
}

// NewInfo return an info component instance
func NewInfo() *InfoBoard {
	c := &InfoBoard{
		Table: tview.NewTable(),
	}
	return c
}

// Init info component init
func (board *InfoBoard) Init(restConf *rest.Config) {
	var row int
	info := model.NewInfo()
	board.SetCell(row, 0, board.sectionCell("Context").SetTextColor(config.InfoSectionColor))
	board.SetCell(row, 1, infoCell(info.CurrentContext()).SetTextColor(config.InfoTextColor))
	row++

	board.SetCell(row, 0, board.sectionCell("Cluster Num").SetTextColor(config.InfoSectionColor))
	board.SetCell(row, 1, infoCell(info.ClusterNum()).SetTextColor(config.InfoTextColor))
	row++

	k8s := model.K8SVersion(restConf)
	board.SetCell(row, 0, board.sectionCell("K8S Version").SetTextColor(config.InfoSectionColor))
	board.SetCell(row, 1, infoCell(k8s).SetTextColor(config.InfoTextColor))
	row++

	velaCLI := model.VelaCLIVersion()
	board.SetCell(row, 0, board.sectionCell("VelaCLI Version").SetTextColor(config.InfoSectionColor))
	board.SetCell(row, 1, infoCell(velaCLI).SetTextColor(config.InfoTextColor))
	row++

	velaCore := model.VelaCoreVersion()
	board.SetCell(row, 0, board.sectionCell("VelaCore Version").SetTextColor(config.InfoSectionColor))
	board.SetCell(row, 1, infoCell(velaCore).SetTextColor(config.InfoTextColor))
	row++

	goVersion := model.GOLangVersion()
	board.SetCell(row, 0, board.sectionCell("Golang Version").SetTextColor(config.InfoSectionColor))
	board.SetCell(row, 1, infoCell(goVersion).SetTextColor(config.InfoTextColor))
}

func (board *InfoBoard) sectionCell(t string) *tview.TableCell {
	c := tview.NewTableCell(t + ":")
	c.SetAlign(tview.AlignLeft)
	return c
}

func infoCell(t string) *tview.TableCell {
	c := tview.NewTableCell(t)
	return c
}

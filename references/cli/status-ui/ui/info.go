package ui

import (
	"github.com/rivo/tview"
	"k8s.io/client-go/rest"

	"github.com/oam-dev/kubevela/references/cli/status-ui/model"
)

type InfoBoard struct {
	*tview.Table
}

func NewInfo() *InfoBoard {
	c := &InfoBoard{
		Table: tview.NewTable(),
	}
	return c
}

func (board *InfoBoard) Init(config *rest.Config) {
	var row int
	info := model.NewInfo()
	board.SetCell(row, 0, board.sectionCell("Cluster"))
	board.SetCell(row, 1, infoCell(info.Cluster()))
	row++

	k8s := info.K8SVersion(config)
	board.SetCell(row, 0, board.sectionCell("K8S Version"))
	board.SetCell(row, 1, infoCell(k8s))
	row++

	velaCLI := info.VelaCLIVersion()
	board.SetCell(row, 0, board.sectionCell("VelaCLI Version"))
	board.SetCell(row, 1, infoCell(velaCLI))
	row++

	velaCore := info.VelaCoreVersion()
	board.SetCell(row, 0, board.sectionCell("VelaCore Version"))
	board.SetCell(row, 1, infoCell(velaCore))
	row++
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

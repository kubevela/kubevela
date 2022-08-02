package ui

import (
	"fmt"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

var VelaLogo = []string{
	`    _  __     _        __     __   _       `,
	`   | |/ /   _| |__   __\ \   / /__| | __ _ `,
	`   | ' / | | | '_ \ / _ \ \ / / _ \ |/ _\ |`,
	`   | . \ |_| | |_) |  __/\ V /  __/ | (_| |`,
	`   |_|\_\__,_|_.__/ \___| \_/ \___|_|\__,_|`,
	`                                           `,
}

type Logo struct {
	*tview.TextView
}

// NewLogo returns a new logo.
func NewLogo() *Logo {
	l := Logo{
		TextView: tview.NewTextView(),
	}
	l.init()
	return &l
}

func (l *Logo) init() {
	l.SetWrap(false)
	l.SetWordWrap(false)
	l.SetTextAlign(tview.AlignCenter)
	l.SetTextColor(tcell.ColorDodgerBlue)
	l.SetDynamicColors(true)

	for _, line := range VelaLogo {
		fmt.Fprintf(l, "%s\n", line)
	}
}

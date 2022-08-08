package component

import (
	"fmt"

	"github.com/rivo/tview"

	"github.com/oam-dev/kubevela/references/cli/top/config"
)

var velaLogo = []string{
	` _  __       _          __     __     _	    `,
	`| |/ /_   _ | |__    ___\ \   / /___ | |  __ _ `,
	`| ' /| | | || '_ \  / _ \\ \ / // _ \| | / _\ |`,
	`| . \| |_| || |_) ||  __/ \ V /|  __/| || (_| |`,
	`|_|\_\\__,_||_.__/  \___|  \_/  \___||_| \__,_|`,
}

// Logo logo component in header
type Logo struct {
	*tview.TextView
}

// NewLogo return logo ui component
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
	l.SetTextColor(config.LogoTextColor)
	l.SetDynamicColors(true)
	for _, line := range velaLogo {
		fmt.Fprintf(l, "%s\n", line)
	}
}

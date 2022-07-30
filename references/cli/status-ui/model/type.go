package model

import "github.com/rivo/tview"

type (
	Component interface {
		Primitive
		Initer
		Hinter
	}

	Primitive interface {
		tview.Primitive
		Name() string
	}

	Initer interface {
		Start()

		Stop()

		Init()
	}

	Hinter interface {
		Hint() []MenuHint
	}
)

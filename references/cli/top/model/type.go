package model

import "github.com/rivo/tview"

type (
	// Component is an abstract of component of app
	Component interface {
		Primitive
		Initer
		Hinter
	}
	// Primitive is an abstract of tview ui component
	Primitive interface {
		tview.Primitive
		// Name return name of the component
		Name() string
	}

	// Initer is an abstract of components whose need to init
	Initer interface {
		// Start the component
		Start()
		// Stop the component
		Stop()
		// Init the component
		Init()
	}

	// Hinter is an abstract of components which can provide menu hints to menu component
	Hinter interface {
		// Hint return key action menu hints of the component
		Hint() []MenuHint
	}
)

var (
	// CtxKeyAppName request context key of application name
	CtxKeyAppName = "appName"
	// CtxKeyCluster request context key of cluster name
	CtxKeyCluster = "cluster"
	// CtxKeyNamespace request context key of namespace name
	CtxKeyNamespace = "appNs"
)

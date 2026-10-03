// CUE that references parameter (not allowed in _imports.cue)

// _imports.cue is compiled as plain CUE with nothing else in scope, so a
// reference to parameter.* does not resolve.
imports: [{
	module: "widget-kit"
	sources: [{registry: "e2e-modules", version: parameter.moduleVersion}]
}]

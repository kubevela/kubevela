// Two modules in one addon. Each entry becomes its own type: module component
// (widget-kit, gadget-kit) and therefore its own owned Application
// (module-widget-kit, module-gadget-kit). The two do not depend on each
// other; both depend on kit-suite-resources.
imports: [{
	module: "widget-kit"
	sources: [{registry: "e2e-modules", version: "1.0.0"}]
}, {
	module: "gadget-kit"
	sources: [{registry: "e2e-modules", version: "1.0.0"}]
}]

// Two crates in one addon. Each entry becomes its own type: crate component
// (widget-kit, gadget-kit) and therefore its own owned Application
// (crate-widget-kit, crate-gadget-kit). The two do not depend on each
// other; both depend on kit-suite-resources.
imports: [{
	crate: "widget-kit"
	sources: [{registry: "e2e-crates", version: "1.0.0"}]
}, {
	crate: "gadget-kit"
	sources: [{registry: "e2e-crates", version: "1.0.0"}]
}]

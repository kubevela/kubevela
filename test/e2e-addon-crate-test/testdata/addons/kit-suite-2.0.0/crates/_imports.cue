// kit-suite 2.0.0 drops gadget-kit. Upgrading an install from 1.0.0 removes
// the gadget-kit component from addon-kit-suite, which garbage-collects
// crate-gadget-kit and everything it installed (definitions, WidgetClass-like
// line objects, the Gadget CRD and therefore every Gadget in the cluster).
imports: [{
	crate: "widget-kit"
	sources: [{registry: "e2e-crates", version: "1.0.0"}]
}]

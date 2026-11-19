imports: [{
	// No `registry`: the module component gets no registry property, and the
	// fetch picks the sole configured module registry (or the one named
	// "catalog" when several exist).
	module: "widget-kit"
	sources: [{
		version: "1.0.0"
		// Accepted but NOT enforced on this branch: the controller logs a
		// warning and installs every enabled line (v1 AND v2).
		versions: ["v1"]
	}]
}, {
	// Skipped entirely: no component is generated.
	module:  "gadget-kit"
	enabled: false
	sources: [{registry: "e2e-modules", version: "1.0.0"}]
}]

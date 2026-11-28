imports: [{
	// No `registry`: the crate component gets no registry property, and the
	// fetch picks the sole configured crate registry (or the one named
	// "catalog" when several exist).
	crate: "widget-kit"
	sources: [{
		version: "1.0.0"
		// Accepted but NOT enforced on this branch: the controller logs a
		// warning and installs every enabled line (v1 AND v2).
		versions: ["v1"]
	}]
}, {
	// Skipped entirely: no component is generated.
	crate:  "gadget-kit"
	enabled: false
	sources: [{registry: "e2e-crates", version: "1.0.0"}]
}]

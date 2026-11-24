// two sources for one crate

imports: [{
	crate: "widget-kit"
	sources: [
		{registry: "e2e-crates", version: "1.0.0"},
		{registry: "e2e-crates", version: "1.1.0"},
	]
}]

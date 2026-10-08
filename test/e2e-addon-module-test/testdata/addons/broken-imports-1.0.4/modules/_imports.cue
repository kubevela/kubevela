// semver range instead of an exact version

// A range is not resolved: it is used verbatim as the OCI tag.
imports: [{
	module: "widget-kit"
	sources: [{registry: "e2e-modules", version: "~1.0.0"}]
}]

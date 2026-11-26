// semver range instead of an exact version

// A range is not resolved: it is used verbatim as the OCI tag.
imports: [{
	crate: "widget-kit"
	sources: [{registry: "e2e-crates", version: "~1.0.0"}]
}]

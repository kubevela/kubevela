// No version: the type: crate component gets no `version` property, and the
// crate fetch resolves the highest semver tag on every reconcile (the
// revision probe makes an unchanged tag cheap).
imports: [{
	crate: "widget-kit"
	sources: [{registry: "e2e-crates"}]
}]

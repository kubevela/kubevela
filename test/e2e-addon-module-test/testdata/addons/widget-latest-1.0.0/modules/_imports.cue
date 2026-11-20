// No version: the type: module component gets no `version` property, and the
// module fetch resolves the highest semver tag on every reconcile (the
// revision probe makes an unchanged tag cheap).
imports: [{
	module: "widget-kit"
	sources: [{registry: "e2e-modules"}]
}]

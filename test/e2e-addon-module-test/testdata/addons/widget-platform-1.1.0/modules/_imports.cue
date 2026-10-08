// External module imports (pkg/addon/module_import.go).
//
// Rules on this branch:
//   - literal CUE only: no parameter.* or context.* references
//   - `module` is required; `enabled` defaults to true
//   - exactly ONE entry in `sources`
//   - a source reads only `registry` (a vela module registry name; empty
//     means the sole registry or the one called "catalog") and `version` (an
//     exact OCI tag; empty means the highest semver tag). `versions` is
//     accepted but only logged; oci/git/apiLine are ignored.
// Each enabled entry becomes a `type: module` component named after the
// module, depending on the addon's resource and outputs components.
imports: [{
	module: "widget-kit"
	sources: [{
		registry: "e2e-modules"
		version:  "1.1.0"
	}]
}]

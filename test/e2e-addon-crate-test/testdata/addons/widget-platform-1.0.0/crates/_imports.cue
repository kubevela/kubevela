// External crate imports (pkg/addon/crate_import.go).
//
// Rules on this branch:
//   - literal CUE only: no parameter.* or context.* references
//   - `crate` is required; `enabled` defaults to true
//   - exactly ONE entry in `sources`
//   - a source reads only `registry` (a vela crate registry name; empty
//     means the sole registry or the one called "catalog") and `version` (an
//     exact OCI tag; empty means the highest semver tag). `versions` is
//     accepted but only logged; oci/git/apiLine are ignored.
// Each enabled entry becomes a `type: crate` component named after the
// crate, depending on the addon's resource and outputs components.
imports: [{
	crate: "widget-kit"
	sources: [{
		registry: "e2e-crates"
		version:  "1.0.0"
	}]
}]

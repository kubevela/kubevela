// Line-level auxiliary resources live in <line>/auxiliary/.
//
// They belong to this API line only. The renderer emits them as their own
// k8s-objects component named "<module>-<apiVersion>-aux", which depends on
// the module-level "<module>-aux" tier, and which this line's definitions
// tier in turn depends on. Lines are siblings: v2's auxiliary does NOT wait
// on v1's.
//
// An auxiliary .cue file is a plain Kubernetes object at the top level --
// apiVersion, kind, metadata and so on written directly, NOT wrapped under a
// named key and NOT a vela definition. (That wrapping is only for files under
// definitions/.) One .cue file is exactly one object; use .yaml when you want
// several in one file.
apiVersion: "v1"
kind:       "ConfigMap"
metadata: name: "demo-store-v1-settings"
data: {
	// Proves this object came from the v1 line's own auxiliary tier.
	tier: "line"
	line: "v1"
}

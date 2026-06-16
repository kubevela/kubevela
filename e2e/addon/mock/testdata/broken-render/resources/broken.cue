output: {
	apiVersion: "v1"
	kind:       "ConfigMap"
	metadata: name: "broken-render-cm"
	// Reference to an identifier that is never defined. This fails at CUE
	// render/eval time (not at fetch/parse), so the addon controller takes the
	// non-fetch InstallFailed branch (SourceResolved=True). Version-agnostic,
	// unlike list-addition which only errors on CUE >= v0.11.
	data: value: _undefinedRenderInput
}

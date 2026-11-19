import "vela/http"

methods: {
	type: "component"
	attributes: workload: definition: {apiVersion: "v1", kind: "ConfigMap"}
}
template: {
	_version: http.#Get & {$params: url: "https://example.com/version"}
	_report: http.#Post & {$params: {url: "https://example.com/report", request: body: "hi"}}
	output: {
		apiVersion: "v1"
		kind:       "ConfigMap"
		data: {
			version: _version.$returns.body
			report:  _report.$returns.body
		}
	}
	parameter: {}
}

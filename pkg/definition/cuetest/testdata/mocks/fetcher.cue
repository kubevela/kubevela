import "vela/http"

fetcher: {
	type: "component"
	attributes: workload: definition: {
		apiVersion: "v1"
		kind:       "ConfigMap"
	}
}
template: {
	resp: http.#Do & {$params: url: parameter.url}
	output: {
		apiVersion: "v1"
		kind:       "ConfigMap"
		data: {status: "\(resp.$returns.statusCode)", body: resp.$returns.body}
	}
	parameter: url: string
}

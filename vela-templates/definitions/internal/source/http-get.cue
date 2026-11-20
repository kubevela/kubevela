import (
	"encoding/json"
	"encoding/yaml"
	"strings"
	"vela/http"
)

"http-get": {
	type: "source"
	annotations: {}
	labels: {}
	description: "GETs a URL. The response is parsed when Content-Type says JSON or YAML; a non-2xx fails the source rather than becoming its value."
}

template: {
	schema: {
		content:    _
		statusCode: int
	}

	storage: {
		storageTTL:     "5m"
		onStaleFailure: "use-stale"
	}

	parameter: {
		// +usage=The URL to GET
		url: string
	}

	_res: http.#Get & {
		$params: url: parameter.url
	}

	_status: _res.$returns.statusCode
	_body:   *_res.$returns.body | ""

	errs: [
		if _status < 200 || _status > 299 {
			"GET \(parameter.url) returned \(_status)"
		},
	]

	_contentType: *strings.ToLower(_res.$returns.header["Content-Type"][0]) | ""

	_isJSON: strings.Contains(_contentType, "json")
	_isYAML: strings.Contains(_contentType, "yaml") || strings.Contains(_contentType, "yml")

	output: {
		statusCode: _status
		content: [
			if _isJSON {json.Unmarshal(_body)},
			if _isYAML {yaml.Unmarshal(_body)},
			_body,
		][0]
	}
}

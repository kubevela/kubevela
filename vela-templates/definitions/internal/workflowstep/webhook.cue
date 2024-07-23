import (
	"vela/op"
	"encoding/json"
	"encoding/base64"
)

"webhook": {
	type: "workflow-step"
	annotations: {
		"category": "External Intergration"
	}
	labels: {}
	description: "Send a POST request to the specified Webhook URL. If no request body is specified, the current Application body will be sent by default."
}
template: {
	data: op.#Steps & {
		if parameter.data == _|_ {
			read: op.#Read & {
				value: {
					apiVersion: "core.oam.dev/v1beta1"
					kind:       "Application"
					metadata: {
						name:      context.name
						namespace: context.namespace
					}
				}
			}
			value: json.Marshal(read.value)
		}
		if parameter.data != _|_ {
			value: json.Marshal(parameter.data)
		}
	}
	webhook: op.#Steps & {
		if parameter.url.value != _|_ {
			http: op.#HTTPPost & {
				url: parameter.url.value
				request: {
					body: data.value
					header: "Content-Type": "application/json"
				}
			}
		}
		if parameter.url.secretRef != _|_ && parameter.url.value == _|_ {
			read: op.#Read & {
				value: {
					apiVersion: "v1"
					kind:       "Secret"
					metadata: {
						name:      parameter.url.secretRef.name
						namespace: context.namespace
					}
				}
			}

			stringValue: op.#ConvertString & {bt: base64.Decode(null, read.value.data[parameter.url.secretRef.key])}
			http:        op.#HTTPPost & {
				url: stringValue.str
				request: {
					body: data.value
					header: "Content-Type": "application/json"
				}
			}
		}
	}

	parameter: {
		// +usage=Specify the webhook url
		url: close({
			value: string
		}) | close({
			secretRef: {
				// +usage=name is the name of the secret
				name: string
				// +usage=key is the key in the secret
				key: string
			}
		})
		// +usage=Specify the data you want to send
		data?: {...}
	}
}

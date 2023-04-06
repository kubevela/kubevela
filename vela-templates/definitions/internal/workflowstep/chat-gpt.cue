import (
	"vela/op"
	"encoding/json"
	"encoding/base64"
)

"chat-gpt": {
	alias: ""
	attributes: {}
	description: "Send request to chat-gpt"
	annotations: {
		"definition.oam.dev/example-url": "https://raw.githubusercontent.com/kubevela/workflow/main/examples/workflow-run/chat-gpt.yaml"
		"category":                       "External Intergration"
	}
	labels: {}
	type: "workflow-step"
}

template: {
	token: op.#Steps & {
		if parameter.token.value != _|_ {
			value: parameter.token.value
		}
		if parameter.token.secretRef != _|_ {
			read: op.#Read & {
				value: {
					apiVersion: "v1"
					kind:       "Secret"
					metadata: {
						name:      parameter.token.secretRef.name
						namespace: context.namespace
					}
				}
			}

			stringValue: op.#ConvertString & {bt: base64.Decode(null, read.value.data[parameter.token.secretRef.key])}
			value:       stringValue.str
		}
	}
	http: op.#HTTPDo & {
		method: "POST"
		url:    "https://api.openai.com/v1/chat/completions"
		request: {
			timeout: parameter.timeout
			body:    json.Marshal({
				model: parameter.model
				messages: [{
					if parameter.prompt.type == "custom" {
						content: parameter.prompt.content
					}
					if parameter.prompt.type == "diagnose" {
						content: """
You are a professional kubernetes administrator.
Carefully read the provided information, being certain to spell out the diagnosis & reasoning, and don't skip any steps.
Answer in  \(parameter.prompt.lang).
---
\(json.Marshal(parameter.prompt.content))
---
What is wrong with this object and how to fix it?
"""
					}
					if parameter.prompt.type == "audit" {
						content: """
You are a professional kubernetes administrator.
You inspect the object and find out the security misconfigurations and give advice.
Write down the possible problems in bullet points, using the imperative tense.
Remember to write only the most important points and do not write more than a few bullet points.
Answer in  \(parameter.prompt.lang).
---
\(json.Marshal(parameter.prompt.content))
---
What is the secure problem with this object and how to fix it?
"""
					}
					if parameter.prompt.type == "quality-gate" {
						content: """
You are a professional kubernetes administrator.
You inspect the object and find out the security misconfigurations and rate the object. The max score is 100.
Answer with score only.
---
\(json.Marshal(parameter.prompt.content))
---
What is the score of this object?
"""
					}
					role: "user"
				}]
			})
			header: {
				"Content-Type":  "application/json"
				"Authorization": "Bearer \(token.value)"
			}
		}
	}
	response: json.Unmarshal(http.response.body)
	fail:     op.#Steps & {
		if http.response.statusCode >= 400 {
			requestFail: op.#Fail & {
				message: "\(http.response.statusCode): failed to request: \(response.error.message)"
			}
		}
	}
	result: response.choices[0].message.content
	log:    op.#Log & {
		data: result
	}
	parameter: {
		token: close({
			// +usage=the token value
			value: string
		}) | close({
			secretRef: {
				// +usage=name is the name of the secret
				name: string
				// +usage=key is the token key in the secret
				key: string
			}
		})
		// +usage=the model name
		model: *"gpt-3.5-turbo" | string
		// +usage=the prompt to use
		prompt: {
			type:    *"custom" | "diagnose" | "audit" | "quality-gate"
			lang:    *"English" | "Chinese"
			content: string | {...}
		}
		timeout: *"30s" | string
	}
}

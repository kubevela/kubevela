import (
	"encoding/yaml"
	"encoding/json"
	"encoding/base64"
	"strings"
)

#ConditionalWait: {
	#do:      "wait"
	continue: bool
}

#Break: {
	#do:     "break"
	message: string
}

#Apply: kube.#Apply

#ApplyApplication: #Steps & {
	load:       oam.#LoadComponets @step(1)
	components: #Steps & {
		for name, c in load.value {
			"\(name)": oam.#Apply & {
				value: c
			}
		}
	} @step(2)
}

#ApplyComponent: oam.#ApplyComponent

#ApplyRemaining: #Steps & {
	// exceptions specify the resources not to apply.
	exceptions: [...string]
	_exceptions: {for c in exceptions {"\(c)": true}}

	load:       ws.#Load @step(1)
	components: #Steps & {
		for name, c in load.value {
			if _exceptions[name] == _|_ {
				"\(name)": oam.#Apply & {
					value: c
				}
			}

		}
	} @step(2)
}

#DingTalk: #Steps & {
	message: dingDing.#DingMessage
	dingUrl: string
	do:      http.#Do & {
		method: "POST"
		url:    dingUrl
		request: {
			body: json.Marshal(message)
			header: "Content-Type": "application/json"
		}
	}
}

#Slack: #Steps & {
	message:  slack.#SlackMessage
	slackUrl: string
	do:       http.#Do & {
		method: "POST"
		url:    slackUrl
		request: {
			body: json.Marshal(message)
			header: "Content-Type": "application/json"
		}
	}
}

#ApplyEnvBindApp: #Steps & {
	env:        string
	policy:     string
	app:        string
	namespace:  string
	_namespace: namespace

	envBinding: kube.#Read & {
		value: {
			apiVersion: "core.oam.dev/v1alpha1"
			kind:       "EnvBinding"
			metadata: {
				name:      policy
				namespace: _namespace
			}
		}
	} @step(1)

	// wait until envBinding.value.status equal "finished"
	wait: #ConditionalWait & {
		continue: envBinding.value.status.phase == "finished"
	} @step(2)

	configMap: kube.#Read & {
		value: {
			apiVersion: "v1"
			kind:       "ConfigMap"
			metadata: {
				name:      policy
				namespace: _namespace
			}
			data?: _
		}
	} @step(3)

	target: "\(policy)-\(env)-\(app)"
	apply:  kube.#Apply & {
		value: {
			yaml.Unmarshal(configMap.value.data[target])
		}
	} @step(4)

	if apply.value.kind == "Application" {
		"wait-app": #ConditionalWait & {
			continue: apply.value.status.status == "running"
		} @step(5)
	}

	if apply.value.kind == "ManifestWork" {
		"wait-manifestWork": #ConditionalWait & {
			continue: len(apply.value.status.resourceStatus) != 0
		} @step(6)

		for manifest in apply.value.status.resourceStatus.manifests {
			for condition in manifest.conditions {
				"wait-\(manifest.resourceMeta.kind)-\(condition.reason)": #ConditionalWait & {
					continue: condition.status == "True"
				} @step(7)
			}
		}
	}
}

#HTTPGet: http.#Do & {method: "GET"}

#HTTPPost: http.#Do & {method: "POST"}

#HTTPPut: http.#Do & {method: "PUT"}

#HTTPDelete: http.#Do & {method: "DELETE"}

#Load: oam.#LoadComponets

#Read: kube.#Read

#Steps: {
	#do: "steps"
	...
}

#Task: task.#Task

NoExist: _|_

context: _

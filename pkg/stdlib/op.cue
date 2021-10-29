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

#Read: kube.#Read

#List: kube.#List

#Delete: kube.#Delete

#ApplyApplication: #Steps & {
	load:       oam.#LoadComponets @step(1)
	components: #Steps & {
		for name, c in load.value {
			"\(name)": oam.#ApplyComponent & {
				value: c
			}
		}
	} @step(2)
}

#ApplyComponent: oam.#ApplyComponent

#RenderComponent: oam.#RenderComponent

#ApplyComponentRemaining: #Steps & {
	// exceptions specify the resources not to apply.
	exceptions: [...string]
	_exceptions: {for c in exceptions {"\(c)": true}}
	component: string

	load:   oam.#LoadComponets @step(1)
	render: #Steps & {
		rendered: oam.#RenderComponent & {
			value: load.value[component]
		}
		comp: kube.#Apply & {
			value: rendered.output
		}
		for name, c in rendered.outputs {
			if _exceptions[name] == _|_ {
				"\(name)": kube.#Apply & {
					value: c
				}
			}
		}
	} @step(2)
}

#ApplyRemaining: #Steps & {
	// exceptions specify the resources not to apply.
	exceptions: [...string]
	_exceptions: {for c in exceptions {"\(c)": true}}

	load:       oam.#LoadComponets @step(1)
	components: #Steps & {
		for name, c in load.value {
			if _exceptions[name] == _|_ {
				"\(name)": oam.#ApplyComponent & {
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

	patchedApp: yaml.Unmarshal(configMap.value.data["\(env)"])[context.name]
	components: patchedApp.spec.components
	apply:      #Steps & {
		for key, comp in components {
			"\(key)": #ApplyComponent & {
				value: comp
				if patchedApp.metadata.labels != _|_ && patchedApp.metadata.labels["cluster.oam.dev/clusterName"] != _|_ {
					cluster: patchedApp.metadata.labels["cluster.oam.dev/clusterName"]
				}
				if patchedApp.metadata.labels != _|_ && patchedApp.metadata.labels["envbinding.oam.dev/override-namespace"] != _|_ {
					namespace: patchedApp.metadata.labels["envbinding.oam.dev/override-namespace"]
				}
			} @step(4)
		}
	}
}

#HTTPGet: http.#Do & {method: "GET"}

#HTTPPost: http.#Do & {method: "POST"}

#HTTPPut: http.#Do & {method: "PUT"}

#HTTPDelete: http.#Do & {method: "DELETE"}

#ConvertString: convert.#String

#SendEmail: email.#Send

#Load: oam.#LoadComponets

#Steps: {
	#do: "steps"
	...
}

#Task: task.#Task

NoExist: _|_

context: _

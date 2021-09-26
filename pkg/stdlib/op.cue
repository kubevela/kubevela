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
			"\(name)": oam.#ApplyComponent & {
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

	target: yaml.Unmarshal(configMap.value.data["\(env)"])
	apply:  #Steps & {
		for key, val in target {
			applyCompRev: string
			"\(key)":     kube.#Apply & {
				value: val
				if val.metadata.labels != _|_ && val.metadata.labels["cluster.oam.dev/clusterName"] != _|_ {
					cluster: val.metadata.labels["cluster.oam.dev/clusterName"]
				}
			} @step(4)

			if val.metadata.labels != _|_ && val.metadata.labels["trait.oam.dev/rely-on-comp-rev"] != _|_ {
				applyCompRev: val.metadata.labels["trait.oam.dev/rely-on-comp-rev"]
			}

			if applyCompRev != _|_ {

				compRev: kube.#Read & {
					value: {
						apiVersion: "apps/v1"
						kind:       "ControllerRevision"
						metadata: {
							name:      applyCompRev
							namespace: _namespace
						}
					}
				} @step(5)

				dispatch: kube.#Apply & {
					value:   compRev
					 if val.metadata.labels != _|_ && val.metadata.labels["cluster.oam.dev/clusterName"] != _|_ {
						 cluster: val.metadata.labels["cluster.oam.dev/clusterName"]
           }
				} @step(6)
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

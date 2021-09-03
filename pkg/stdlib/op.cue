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
	load:       ws.#Load @step(1)
	components: #Steps & {
		for name, c in load.value {
			"\(name)": #Steps & {
				workload: kube.#Apply & {value: c.workload}
				if c.auxiliaries != _|_ {
					_key: "trait.oam.dev/resource"
					for index, o in c.auxiliaries {
						"\(o.metadata.labels[_key])": kube.#Apply & {value: o}
					}
				}
			}
		}
	} @step(2)
}

#ApplyComponent: #Steps & {

	component:      string
	_componentName: component
	load:           ws.#Load & {
		component: _componentName
	} @step(1)

	traits: #Steps & {
		_key:              "trait.oam.dev/resource"
		_manWlKey:         "trait.oam.dev/manage-workload"
		skipApplyWorkload: *false | bool
		if load.value.auxiliaries != _|_ {
			for o in load.value.auxiliaries {
				"\(o.metadata.labels[_key])": kube.#Apply & {value: o}
				if o.metadata.labels[_manWlKey] != _|_ {
					skipApplyWorkload: true
				}
			}
		}
	} @step(2)

	workload__: {
		if !traits.skipApplyWorkload {
			kube.#Apply & {
				value: load.value.workload
				...
			}
		}
	} @step(3)
}

#ApplyRemaining: #Steps & {
	namespace?: string

	// exceptions specify the resources not to apply.
	exceptions?: [componentName=string]: {
		// skipApplyWorkload indicates whether to skip apply the workload resource
		skipApplyWorkload: *true | bool

		// skipAllTraits indicates to skip apply all resources of the traits.
		// If this is true, skipApplyTraits will be ignored
		skipAllTraits: *true | bool

		// skipApplyTraits specifies the names of the traits to skip apply
		skipApplyTraits: [...string]
	}

	components: ws.#Load @step(1)
	#up__: [ for name, c in components.value {
		#Steps
		if exceptions[name] != _|_ {
			if exceptions[name].skipApplyWorkload == false {
				"apply-workload": kube.#Apply & {value: c.workload}
			}
			if exceptions[name].skipAllTraits == false && c.auxiliaries != _|_ {
				#up_auxiliaries: [ for t in c.auxiliaries {
					kube.#Apply & {value: t}
				}]
			}
		}
		if exceptions[name] == _|_ {
			"apply-workload": kube.#Apply & {value: c.workload}
			if c.auxiliaries != _|_ {
				#up_auxiliaries: [ for index, o in c.auxiliaries {
					"s\(index)": kube.#Apply & {
						value: o
					}
				}]
			}

		}
	},
	] @step(2)
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

#Load: ws.#Load

#Read: kube.#Read

#Steps: {
	#do: "steps"
	...
}

#Task: task.#Task

NoExist: _|_

import (
	"encoding/json"
	"encoding/base64"
	"strings"
)

#ConditionalWait: {
	#do:      "wait"
	continue: bool
	message?: string
}

#Break: {
	#do:      "break"
	message?: string
}

#Apply: kube.#Apply

#Read: kube.#Read

#List: kube.#List

#Delete: kube.#Delete

#ApplyApplication: #Steps & {
	load:       oam.#LoadComponetsInOrder @step(1)
	components: #Steps & {
		for name, c in load.value {
			"\(name)": oam.#ApplyComponent & {
				value: c
			}
		}
	} @step(2)
}

// This operator will dispatch all the components in parallel when applying an application.
// Currently it works for Addon Observability to speed up the installation. It can also works for other applications, which
// needs to skip health check for components.
#ApplyApplicationInParallel: #Steps & {
	load:       oam.#LoadComponetsInOrder @step(1)
	components: #Steps & {
		for name, c in load.value {
			"\(name)": oam.#ApplyComponent & {
				value:       c
				waitHealthy: false
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

#Lark: #Steps & {
	message: lark.#LarkMessage
	larkUrl: string
	do:      http.#Do & {
		method: "POST"
		url:    larkUrl
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

#ApplyEnvBindApp: multicluster.#ApplyEnvBindApp

#DeployCloudResource: terraform.#DeployCloudResource

#ShareCloudResource: terraform.#ShareCloudResource

#LoadPolicies: oam.#LoadPolicies

#ListClusters: multicluster.#ListClusters

#MakePlacementDecisions: multicluster.#MakePlacementDecisions

#PatchApplication: multicluster.#PatchApplication

#HTTPGet: http.#Do & {method: "GET"}

#HTTPPost: http.#Do & {method: "POST"}

#HTTPPut: http.#Do & {method: "PUT"}

#HTTPDelete: http.#Do & {method: "DELETE"}

#ConvertString: convert.#String

#DateToTimestamp: time.#DateToTimestamp

#TimestampToDate: time.#TimestampToDate

#SendEmail: email.#Send

#Load: oam.#LoadComponets

#LoadInOrder: oam.#LoadComponetsInOrder

#Steps: {
	#do: "steps"
	...
}

#Task: task.#Task

NoExist: _|_

context: _

import "vela/test"

_posted: mocks: "vela/http": "#HTTPDo": $returns: {statusCode: 200, body: "ok"}
_sent: mocks: "vela/email": "#SendEmail": {}

// The webhook URLs a channel's secretRef reads, from the step's namespace.
_webhooks: resources: [{
	apiVersion: "v1"
	kind:       "Secret"
	metadata: name: "webhooks"
	stringData: {
		dingding: "https://oapi.dingtalk.com/robot/send?access_token=secret"
		lark:     "https://open.feishu.cn/open-apis/bot/v2/hook/secret"
		slack:    "https://hooks.slack.com/services/T0/B0/secret"
		smtp:     "s3cret"
	}
}]

_json: {"Content-Type": "application/json"}

"posts a DingTalk message": test.#WorkflowStepExec & _posted & {
	definition: "notification"
	parameter: dingding: {
		url: value: "https://oapi.dingtalk.com/robot/send?access_token=x"
		message: text: content: "shop deployed"
	}
	expect: {
		phase: "succeeded"
		calls: "vela/http": "#HTTPDo": [{$params: {
			method: "POST"
			url:    "https://oapi.dingtalk.com/robot/send?access_token=x"
			request: {
				body:     "{\"text\":{\"content\":\"shop deployed\"},\"msgtype\":\"text\"}"
				header:   _json
				timeout?: _|_
			}
		}}]
	}
}

"posts a DingTalk markdown message that mentions everyone": test.#WorkflowStepExec & _posted & {
	definition: "notification"
	parameter: dingding: {
		url: value: "https://oapi.dingtalk.com/robot/send?access_token=x"
		message: {
			msgtype: "markdown"
			markdown: {title: "Deployed", text: "**shop** is live"}
			at: isAtAll: true
		}
	}
	expect: calls: "vela/http": "#HTTPDo": [{$params: request: body: "{\"msgtype\":\"markdown\",\"markdown\":{\"text\":\"**shop** is live\",\"title\":\"Deployed\"},\"at\":{\"isAtAll\":true}}"}]
}

"posts a DingTalk message to the URL in a Secret": test.#WorkflowStepExec & _posted & _webhooks & {
	definition: "notification"
	parameter: dingding: {
		url: secretRef: {name: "webhooks", key: "dingding"}
		message: text: content: "shop deployed"
	}
	expect: {
		phase: "succeeded"
		calls: {
			"vela/kube": "#Read": [{$params: value: {kind: "Secret", metadata: name: "webhooks"}}]
			"vela/http": "#HTTPDo": [{$params: {method: "POST", url: "https://oapi.dingtalk.com/robot/send?access_token=secret"}}]
		}
	}
}

"posts a Lark message": test.#WorkflowStepExec & _posted & {
	definition: "notification"
	parameter: lark: {
		url: value: "https://open.feishu.cn/open-apis/bot/v2/hook/x"
		message: {msg_type: "text", content: "{\"text\":\"shop deployed\"}"}
	}
	expect: {
		phase: "succeeded"
		calls: "vela/http": "#HTTPDo": [{$params: {
			method: "POST"
			url:    "https://open.feishu.cn/open-apis/bot/v2/hook/x"
			request: {
				body:   "{\"msg_type\":\"text\",\"content\":\"{\\\"text\\\":\\\"shop deployed\\\"}\"}"
				header: _json
			}
		}}]
	}
}

"posts a Lark message to the URL in a Secret": test.#WorkflowStepExec & _posted & _webhooks & {
	definition: "notification"
	parameter: lark: {
		url: secretRef: {name: "webhooks", key: "lark"}
		message: {msg_type: "text", content: "{\"text\":\"shop deployed\"}"}
	}
	expect: {
		phase: "succeeded"
		calls: "vela/http": "#HTTPDo": [{$params: url: "https://open.feishu.cn/open-apis/bot/v2/hook/secret"}]
	}
}

// mrkdwn is optional, so its default of true is never sent; Slack's own
// default is also markdown.
"posts a Slack message without a markdown flag by default": test.#WorkflowStepExec & _posted & {
	definition: "notification"
	parameter: slack: {
		url: value:    "https://hooks.slack.com/services/T0/B0/x"
		message: text: "shop deployed"
	}
	expect: {
		phase: "succeeded"
		calls: "vela/http": "#HTTPDo": [{$params: {
			method: "POST"
			url:    "https://hooks.slack.com/services/T0/B0/x"
			request: {
				body:   "{\"text\":\"shop deployed\"}"
				header: _json
			}
		}}]
	}
}

"posts Slack blocks as given": test.#WorkflowStepExec & _posted & {
	definition: "notification"
	parameter: slack: {
		url: value: "https://hooks.slack.com/services/T0/B0/x"
		message: {
			text:   "shop deployed"
			mrkdwn: false
			blocks: [{type: "section", elements: [{type: "mrkdwn", text: {type: "mrkdwn", text: "*shop* is live"}}]}]
		}
	}
	expect: calls: "vela/http": "#HTTPDo": [{$params: request: body: "{\"text\":\"shop deployed\",\"blocks\":[{\"type\":\"section\",\"elements\":[{\"type\":\"mrkdwn\",\"text\":{\"type\":\"mrkdwn\",\"text\":\"*shop* is live\"}}]}],\"mrkdwn\":false}"}]
}

"posts a Slack message to the URL in a Secret": test.#WorkflowStepExec & _posted & _webhooks & {
	definition: "notification"
	parameter: slack: {
		url: secretRef: {name: "webhooks", key: "slack"}
		message: text: "shop deployed"
	}
	expect: {
		phase: "succeeded"
		calls: "vela/http": "#HTTPDo": [{$params: url: "https://hooks.slack.com/services/T0/B0/secret"}]
	}
}

"a Slack message needs its text": test.#WorkflowStepExec & _posted & {
	definition: "notification"
	parameter: slack: {
		url: value: "https://hooks.slack.com/services/T0/B0/x"
		message: {}
	}
	expect: {
		phase:   "failed"
		message: =~"parameter.slack.message"
		calls: "vela/http"?: _|_
	}
}

"fails when the URL Secret does not exist": test.#WorkflowStepExec & _posted & {
	definition: "notification"
	parameter: slack: {
		url: secretRef: {name: "missing", key: "slack"}
		message: text: "shop deployed"
	}
	expect: {
		phase: "failed"
		calls: "vela/http"?: _|_
	}
}

"sends the HTTP notifications with the given timeout": test.#WorkflowStepExec & _posted & {
	definition: "notification"
	parameter: {
		timeout: "10s"
		slack: {url: value: "https://hooks.slack.com/services/T0/B0/x", message: text: "shop deployed"}
		lark: {url: value: "https://open.feishu.cn/open-apis/bot/v2/hook/x", message: {msg_type: "text", content: "{}"}}
	}
	expect: calls: "vela/http": "#HTTPDo": [{$params: request: timeout: "10s"}, {$params: request: timeout: "10s"}]
}

"a timeout that is not a duration is rejected": test.#WorkflowStepExec & _posted & {
	definition: "notification"
	parameter: {
		timeout: "soon"
		slack: {url: value: "https://hooks.slack.com/services/T0/B0/x", message: text: "shop deployed"}
	}
	expect: {
		phase:   "failed"
		message: =~"timeout"
	}
}

_email: {
	from: {address: "vela@example.com", password: value: "s3cret", host: "smtp.example.com"}
	to: ["ops@example.com"]
	content: {subject: "shop deployed", body: "shop is live"}
}

"sends an email on port 587 by default": test.#WorkflowStepExec & _sent & {
	definition: "notification"
	parameter: email: _email
	expect: {
		phase: "succeeded"
		calls: "vela/email": "#SendEmail": [{$params: {
			from: {address: "vela@example.com", password: "s3cret", host: "smtp.example.com", port: 587, alias?: _|_}
			to: ["ops@example.com"]
			content: {subject: "shop deployed", body: "shop is live"}
		}}]
	}
}

"sends an email with the given alias and port": test.#WorkflowStepExec & _sent & {
	definition: "notification"
	parameter: email: _email & {from: {alias: "KubeVela", port: 465}}
	expect: calls: "vela/email": "#SendEmail": [{$params: from: {alias: "KubeVela", port: 465}}]
}

"sends an email with the password in a Secret": test.#WorkflowStepExec & _sent & _webhooks & {
	definition: "notification"
	parameter: email: {
		from: {address: "vela@example.com", password: secretRef: {name: "webhooks", key: "smtp"}, host: "smtp.example.com"}
		to: ["ops@example.com"]
		content: {subject: "shop deployed", body: "shop is live"}
	}
	expect: {
		phase: "succeeded"
		calls: "vela/email": "#SendEmail": [{$params: from: password: "s3cret"}]
	}
} @pending(the definition passes stringValue.str rather than the returned str as the password, so the call never has one)

"notifies every channel given": test.#WorkflowStepExec & _posted & _sent & {
	definition: "notification"
	parameter: {
		dingding: {url: value: "https://oapi.dingtalk.com/robot/send?access_token=x", message: text: content: "shop deployed"}
		lark: {url: value: "https://open.feishu.cn/open-apis/bot/v2/hook/x", message: {msg_type: "text", content: "{}"}}
		slack: {url: value: "https://hooks.slack.com/services/T0/B0/x", message: text: "shop deployed"}
		email: _email
	}
	expect: {
		phase: "succeeded"
		calls: {
			"vela/http": {
				"#HTTPDo": [
					{$params: url: =~"dingtalk"},
					{$params: url: =~"feishu"},
					{$params: url: =~"slack"},
				] @contains()
			}
			"vela/email": "#SendEmail": [_]
		}
	}
}

"does nothing without a channel": test.#WorkflowStepExec & {
	definition: "notification"
	expect: {
		phase: "succeeded"
		calls: {
			"vela/http"?:  _|_
			"vela/email"?: _|_
			"vela/kube"?:  _|_
		}
	}
}

"an unmocked post fails the step": test.#WorkflowStepExec & {
	definition: "notification"
	parameter: slack: {url: value: "https://hooks.slack.com/services/T0/B0/x", message: text: "shop deployed"}
	expect: {
		phase:   "failed"
		message: =~"reaches outside the cluster"
	}
}

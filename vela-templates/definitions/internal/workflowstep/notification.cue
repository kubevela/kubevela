import (
	"vela/op"
	"encoding/base64"
)

"notification": {
	type: "workflow-step"
	annotations: {}
	labels: {}
	description: "Send message to webhook"
}
template: {

	parameter: {
		dingding?: {
			// +usage=Specify the the dingding url, you can either sepcify it in value or use secretRef
			url: value | secretRef
			// +useage=Specify the message that you want to sent
			message: {
				text?: *null | {
					content: string
				}
				// +usage=msgType can be text, link, mardown, actionCard, feedCard
				msgtype: *"text" | "link" | "markdown" | "actionCard" | "feedCard"
				link?:   *null | {
					text?:       string
					title?:      string
					messageUrl?: string
					picUrl?:     string
				}
				markdown?: *null | {
					text:  string
					title: string
				}
				at?: *null | {
					atMobiles?: *null | [...string]
					isAtAll?:   bool
				}
				actionCard?: *null | {
					text:           string
					title:          string
					hideAvatar:     string
					btnOrientation: string
					singleTitle:    string
					singleURL:      string
					btns:           *null | [...*null | {
						title:     string
						actionURL: string
					}]
				}
				feedCard?: *null | {
					links: *null | [...*null | {
						text?:       string
						title?:      string
						messageUrl?: string
						picUrl?:     string
					}]
				}
			}
		}

		slack?: {
			// +usage=Specify the the slack url, you can either sepcify it in value or use secretRef
			url: value | secretRef
			// +useage=Specify the message that you want to sent
			message: {
				text:         string
				blocks?:      *null | [...block]
				attachments?: *null | {
					blocks?: *null | [...block]
					color?:  string
				}
				thread_ts?: string
				mrkdwn?:    *true | bool
			}
		}

		email?: {
			// +usage=Specify the email info that you want to send from
			from: {
				// +usage=Specify the email address that you want to send from
				address: string
				// +usage=The alias is the email alias to show after sending the email
				alias?: string
				// +usage=Specify the password of the email, you can either sepcify it in value or use secretRef
				password: value | secretRef
				// +usage=Specify the host of your email
				host: string
				// +usage=Specify the port of the email host, default to 587
				port: *587 | int
			}
			// +usage=Specify the email address that you want to send to
			to: [...string]
			// +usage=Specify the content of the email
			content: {
				// +usage=Specify the subject of the email
				subject: string
				// +usage=Specify the context body of the email
				body: string
			}
		}
	}

	block: {
		type:      string
		block_id?: string
		elements?: [...{
			type:       string
			action_id?: string
			url?:       string
			value?:     string
			style?:     string
			text?:      textType
			confirm?: {
				title:   textType
				text:    textType
				confirm: textType
				deny:    textType
				style?:  string
			}
			options?: [...option]
			initial_options?: [...option]
			placeholder?:  textType
			initial_date?: string
			image_url?:    string
			alt_text?:     string
			option_groups?: [...option]
			max_selected_items?: int
			initial_value?:      string
			multiline?:          bool
			min_length?:         int
			max_length?:         int
			dispatch_action_config?: {
				trigger_actions_on?: [...string]
			}
			initial_time?: string
		}]
	}

	textType: {
		type:      string
		text:      string
		emoji?:    bool
		verbatim?: bool
	}

	option: {
		text:         textType
		value:        string
		description?: textType
		url?:         string
	}

	secretRef: {
		// +usage=name is the name of the secret
		name: string
		// +usage=key is the key in the secret
		key: string
	}

	value: string

	// send webhook notification
	ding: op.#Steps & {
		if parameter.dingding != _|_ {
			if parameter.dingding.url.value != _|_ {
				ding1: op.#DingTalk & {
					message: parameter.dingding.message
					dingUrl: parameter.dingding.url.value
				}
			}
			if parameter.dingding.url.secretRef != _|_ && parameter.dingding.url.value == _|_ {
				read: op.#Read & {
					value: {
						apiVersion: "v1"
						kind:       "Secret"
						metadata: {
							name:      parameter.dingding.url.secretRef.name
							namespace: context.namespace
						}
					}
				}

				decoded:     base64.Decode(null, read.value.data[parameter.dingding.url.secretRef.key])
				stringValue: op.#ConvertString & {bt: decoded}
				ding2:       op.#DingTalk & {
					message: parameter.dingding.message
					dingUrl: stringValue.str
				}
			}
		}
	}

	slack: op.#Steps & {
		if parameter.slack != _|_ {
			if parameter.slack.url.value != _|_ {
				slack1: op.#Slack & {
					message:  parameter.slack.message
					slackUrl: parameter.slack.url.value
				}
			}
			if parameter.slack.url.secretRef != _|_ && parameter.slack.url.value == _|_ {
				read: op.#Read & {
					value: {
						kind:       "Secret"
						apiVersion: "v1"
						metadata: {
							name:      parameter.slack.url.secretRef.name
							namespace: context.namespace
						}
					}
				}

				decoded:     base64.Decode(null, read.value.data[parameter.slack.url.secretRef.key])
				stringValue: op.#ConvertString & {bt: decoded}
				slack2:      op.#Slack & {
					message:  parameter.slack.message
					slackUrl: stringValue.str
				}
			}
		}
	}

	email: op.#Steps & {
		if parameter.email != _|_ {
			if parameter.email.from.password.value != _|_ {
				email1: op.#SendEmail & {
					from: {
						address:  parameter.email.from.value
						alias:    parameter.email.from.alias
						password: parameter.email.from.password.value
						host:     parameter.email.from.host
						port:     parameter.email.from.port
					}
					to:      parameter.email.to
					content: parameter.email.content
				}
			}

			if parameter.email.from.password.secretRef != _|_ && parameter.email.from.password.value == _|_ {
				read: op.#Read & {
					value: {
						kind:       "Secret"
						apiVersion: "v1"
						metadata: {
							name:      parameter.email.from.password.secretRef.name
							namespace: context.namespace
						}
					}
				}

				decoded:     base64.Decode(null, read.value.data[parameter.email.from.password.secretRef.key])
				stringValue: op.#ConvertString & {bt: decoded}
				email2:      op.#SendEmail & {
					from: {
						address:  parameter.email.from.value
						alias:    parameter.email.from.alias
						password: stringValue.str
						host:     parameter.email.from.host
						port:     parameter.email.from.port
					}
					to:      parameter.email.to
					content: parameter.email.content
				}
			}
		}
	}
}

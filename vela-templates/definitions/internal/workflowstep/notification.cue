import (
	"vela/op"
	"encoding/base64"
)

"notification": {
	type: "workflow-step"
	annotations: {
		"category": "External Integration"
	}
	labels: {}
	description: "Send notifications to Email, DingTalk, Slack, Lark or webhook in your workflow."
}
template: {

	parameter: {
		// +usage=Please fulfill its url and message if you want to send Lark messages
		lark?: {
			// +usage=Specify the the lark url, you can either sepcify it in value or use secretRef
			url: close({
				// +usage=the url address content in string
				value: string
			}) | close({
				secretRef: {
					// +usage=name is the name of the secret
					name: string
					// +usage=key is the key in the secret
					key: string
				}
			})
			// +usage=Specify the message that you want to sent, refer to [Lark messaging](https://open.feishu.cn/document/ukTMukTMukTM/ucTM5YjL3ETO24yNxkjN#8b0f2a1b).
			message: {
				// +usage=msg_type can be text, post, image, interactive, share_chat, share_user, audio, media, file, sticker
				msg_type: string
				// +usage=content should be json encode string
				content: string
			}
		}
		// +usage=Please fulfill its url and message if you want to send DingTalk messages
		dingding?: {
			// +usage=Specify the the dingding url, you can either sepcify it in value or use secretRef
			url: close({
				// +usage=the url address content in string
				value: string
			}) | close({
				secretRef: {
					// +usage=name is the name of the secret
					name: string
					// +usage=key is the key in the secret
					key: string
				}
			})
			// +usage=Specify the message that you want to sent, refer to [dingtalk messaging](https://developers.dingtalk.com/document/robots/custom-robot-access/title-72m-8ag-pqw)
			message: {
				// +usage=Specify the message content of dingtalk notification
				text?: close({
					content: string
				})
				// +usage=msgType can be text, link, mardown, actionCard, feedCard
				msgtype: *"text" | "link" | "markdown" | "actionCard" | "feedCard"
				#link: {
					text?:       string
					title?:      string
					messageUrl?: string
					picUrl?:     string
				}

				link?:     #link
				markdown?: close({
					text:  string
					title: string
				})
				at?: close({
					atMobiles?: [...string]
					isAtAll?: bool
				})
				actionCard?: close({
					text:           string
					title:          string
					hideAvatar:     string
					btnOrientation: string
					singleTitle:    string
					singleURL:      string
					btns?: [...close({
						title:     string
						actionURL: string
					})]
				})
				feedCard?: close({
					links: [...#link]
				})
			}
		}
		// +usage=Please fulfill its url and message if you want to send Slack messages
		slack?: {
			// +usage=Specify the the slack url, you can either sepcify it in value or use secretRef
			url: close({
				// +usage=the url address content in string
				value: string
			}) | close({
				secretRef: {
					// +usage=name is the name of the secret
					name: string
					// +usage=key is the key in the secret
					key: string
				}
			})
			// +usage=Specify the message that you want to sent, refer to [slack messaging](https://api.slack.com/reference/messaging/payload)
			message: {
				// +usage=Specify the message text for slack notification
				text: string
				blocks?: [...block]
				attachments?: close({
					blocks?: [...block]
					color?: string
				})
				thread_ts?: string
				// +usage=Specify the message text format in markdown for slack notification
				mrkdwn?: *true | bool
			}
		}
		// +usage=Please fulfill its from, to and content if you want to send email
		email?: {
			// +usage=Specify the email info that you want to send from
			from: {
				// +usage=Specify the email address that you want to send from
				address: string
				// +usage=The alias is the email alias to show after sending the email
				alias?: string
				// +usage=Specify the password of the email, you can either sepcify it in value or use secretRef
				password: close({
					// +usage=the password content in string
					value: string
				}) | close({
					secretRef: {
						// +usage=name is the name of the secret
						name: string
						// +usage=key is the key in the secret
						key: string
					}
				})
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

				stringValue: op.#ConvertString & {bt: base64.Decode(null, read.value.data[parameter.dingding.url.secretRef.key])}
				ding2:       op.#DingTalk & {
					message: parameter.dingding.message
					dingUrl: stringValue.str
				}
			}
		}
	}

	lark: op.#Steps & {
		if parameter.lark != _|_ {
			if parameter.lark.url.value != _|_ {
				lark1: op.#Lark & {
					message: parameter.lark.message
					larkUrl: parameter.lark.url.value
				}
			}
			if parameter.lark.url.secretRef != _|_ && parameter.lark.url.value == _|_ {
				read: op.#Read & {
					value: {
						apiVersion: "v1"
						kind:       "Secret"
						metadata: {
							name:      parameter.lark.url.secretRef.name
							namespace: context.namespace
						}
					}
				}

				stringValue: op.#ConvertString & {bt: base64.Decode(null, read.value.data[parameter.lark.url.secretRef.key])}
				lark2:       op.#Lark & {
					message: parameter.lark.message
					larkUrl: stringValue.str
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

				stringValue: op.#ConvertString & {bt: base64.Decode(null, read.value.data[parameter.slack.url.secretRef.key])}
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
						address: parameter.email.from.address
						if parameter.email.from.alias != _|_ {
							alias: parameter.email.from.alias
						}
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

				stringValue: op.#ConvertString & {bt: base64.Decode(null, read.value.data[parameter.email.from.password.secretRef.key])}
				email2:      op.#SendEmail & {
					from: {
						address: parameter.email.from.address
						if parameter.email.from.alias != _|_ {
							alias: parameter.email.from.alias
						}
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

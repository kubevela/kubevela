import (
	"vela/op"
)

"webhook-notification": {
	type: "workflow-step"
	annotations: {}
	labels: {}
	description: "Send message to webhook"
}
template: {

	parameter: {
		dingding?: {
			url: string
			message: {
				text?: *null | {
					content: string
				}
				// +usage=msgType can be text, link, mardown, actionCard, feedCard
				msgtype: string
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
			url: string
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
			text?:      text
			confirm?: {
				title:   text
				text:    text
				confirm: text
				deny:    text
				style?:  string
			}
			options?: [...option]
			initial_options?: [...option]
			placeholder?:  text
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

	text: {
		type:      string
		text:      string
		emoji?:    bool
		verbatim?: bool
	}

	option: {
		text:         text
		value:        string
		description?: text
		url?:         string
	}

	// send webhook notification
	ding: {
		if parameter.dingding != _|_ {
			op.#DingTalk & {
				message: parameter.dingding.message
				dingUrl: parameter.dingding.url
			}
		}
	}

	slack: {
		if parameter.slack != _|_ {
			op.#Slack & {
				message:  parameter.slack.message
				slackUrl: parameter.slack.url
			}
		}
	}
}

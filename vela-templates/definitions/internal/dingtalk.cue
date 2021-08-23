import (
	"vela/op"
)

"dingtalk": {
	type: "workflow-step"
	annotations: {}
	labels: {}
	description: "Send message with DingTalk robot"
}
template: {
	// apply remaining components and traits
	ding: op.#DingTalk & {
		parameter
	}

	parameter: {
		// +usage=Declare the token of the DingTalk robot
		token: string
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
}

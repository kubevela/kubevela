import (
	"vela/op"
	"encoding/base64"
)

"one_of": {
	type:        "workflow-step"
	description: "Send notifications to Email, DingTalk, Slack, Lark or webhook in your workflow. For test one_of"
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
		}
	}
}

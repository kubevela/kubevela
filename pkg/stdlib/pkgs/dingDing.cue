#Message: {
	text?: *null | {
		content: string
	}
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

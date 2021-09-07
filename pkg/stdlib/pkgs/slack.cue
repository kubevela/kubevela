#SlackMessage: {
	text:         string
	blocks?:      *null | [...#block]
	attachments?: *null | {
		blocks?: *null | [...#block]
		color?:  string
	}
	thread_ts?: string
	mrkdwn?:    *true | bool
}

#block: {
	type:      string
	block_id?: string
	elements?: [...{
		type:       string
		action_id?: string
		url?:       string
		value?:     string
		style?:     string
		text?:      #text
		confirm?: {
			title:   #text
			text:    #text
			confirm: #text
			deny:    #text
			style?:  string
		}
		options?: [...#option]
		initial_options?: [...#option]
		placeholder?:  #text
		initial_date?: string
		image_url?:    string
		alt_text?:     string
		option_groups?: [...#option]
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

#text: {
	type:      string
	text:      string
	emoji?:    bool
	verbatim?: bool
}

#option: {
	text:         text
	value:        string
	description?: text
	url?:         string
}

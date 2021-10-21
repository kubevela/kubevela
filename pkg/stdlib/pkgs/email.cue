#Send: {
	#do:       "send"
	#provider: "email"

	sender: {
		address:  string
		alias?:   string
		password: string
		host:     string
		port:     int
	}
	receiver: [...string]
	content: {
		subject: string
		body:    string
	}
	stepID: context.stepSessionID
	...
}

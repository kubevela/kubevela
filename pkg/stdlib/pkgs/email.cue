#Send: {
	#do:       "send"
	#provider: "email"

	from: {
		address:  string
		alias?:   string
		password: string
		host:     string
		port:     int
	}
	to: [...string]
	content: {
		subject: string
		body:    string
	}
	stepID: context.stepSessionID
	...
}

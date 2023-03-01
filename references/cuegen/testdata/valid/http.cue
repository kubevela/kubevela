package test

// RequestVars is the vars for http request
// TODO: support timeout & tls
RequestVars: {
	method: string
	url:    string
	request: {
		body: string
		header: [string]: [...string]
		trailer: [string]: [...string]
	}
}
// ResponseVars is the vars for http response
ResponseVars: {
	body: string
	header: [string]: [...string]
	trailer: [string]: [...string]
	statusCode: int
}
// DoParams is the params for http request
DoParams: $params: {
	method: string
	url:    string
	request: {
		body: string
		header: [string]: [...string]
		trailer: [string]: [...string]
	}
}
// DoReturns returned struct for http response
DoReturns: $returns: {
	body: string
	header: [string]: [...string]
	trailer: [string]: [...string]
	statusCode: int
}

#Do: {
	#do:       "do"
	#provider: "http"

	method: *"GET" | "POST" | "PUT" | "DELETE"
	url:    string
	request?: {
		body: string
		header: [string]:  string
		trailer: [string]: string
	}
	response: {
		body: string
		header?: [string]: [...string]
		trailer?: [string]: [...string]
	}
	...
}

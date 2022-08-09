#Do: {
	#do:       "do"
	#provider: "http"

	method: *"GET" | "POST" | "PUT" | "DELETE"
	url:    string
	request?: {
		body: string
		header: [string]:  string
		trailer: [string]: string
		...
	}
	tls_config?: secret: string
	response: {
		body: string
		header?: [string]: [...string]
		trailer?: [string]: [...string]
		statusCode: number
		...
	}
	...
}

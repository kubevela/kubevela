metadata: {
	name:  "nacos-server"
	alias: "Nacos Server"
}

template: {
	parameter: {
		servers?: [...{
			ipAddr: string
			port:   int
		}]
		client?: {
			endpoint:  string
			accessKey: string
			secretKey: string
		}
	}
}

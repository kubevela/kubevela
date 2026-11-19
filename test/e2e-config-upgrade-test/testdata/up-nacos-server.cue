metadata: {
	name:        "up-nacos-server"
	alias:       "up-nacos-server"
	description: "config upgrade e2e"
	scope:       "system"
	sensitive:   false
}
template: {
	parameter: {
		servers: [...{
			ipAddr:    string
			port:      int
			grpcPort?: int
		}]
	}
}

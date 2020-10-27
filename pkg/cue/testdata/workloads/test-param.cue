Template: {
}

parameter: {
	name: string
	// +usage=specify app image
	// +short=i
	image: string
	// +usage=specify port for container
	// +short=p
	port: *8080 | int
	env: [...{
		name:  string
		value: string
	}]
	enable: *false | bool
	fval:   *64.3 | number
	nval:   number
}

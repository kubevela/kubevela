Template: {
}

parameter: {
	name: string
	// +usage=Which image would you like to use for your service
	// +short=i
	image: string
	// +usage=Which port do you want customer traffic sent to
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

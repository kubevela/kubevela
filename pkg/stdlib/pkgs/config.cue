#Create: {
	#do:       "create"
	#provider: "config"

	name:      string
	namespace: string
	template?: string
	config: {
		...
	}
}

#Delete: {
	#do:       "delete"
	#provider: "config"

	name:      string
	namespace: string
}

#Read: {
	#do:       "delete"
	#provider: "config"

	name:      string
	namespace: string

	config: {...}
}

#List: {
	#do:       "delete"
	#provider: "config"

	// Must query with the template
	template:  string
	namespace: string

	configs: [...{...}]
}

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
	#do:       "read"
	#provider: "config"

	name:      string
	namespace: string

	config: {...}
}

#List: {
	#do:       "list"
	#provider: "config"

	// Must query with the template
	template:  string
	namespace: string

	configs: [...{...}]
}

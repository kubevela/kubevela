// config.cue

#CreateConfig: {
	#do:       "create-config"
	#provider: "op"

	name:      string
	namespace: string
	template?: string
	config: {
		...
	}
}

#DeleteConfig: {
	#do:       "delete-config"
	#provider: "op"

	name:      string
	namespace: string
}

#ReadConfig: {
	#do:       "read-config"
	#provider: "op"

	name:      string
	namespace: string

	config: {...}
}

#ListConfig: {
	#do:       "list-config"
	#provider: "op"

	// Must query with the template
	template:  string
	namespace: string

	configs: [...{...}]
}

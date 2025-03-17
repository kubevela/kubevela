// config.cue

#CreateConfig: {
	#do:       "create"
	#provider: "op"

	name:      string
	namespace: string
	template?: string
	config: {
		...
	}
}

#DeleteConfig: {
	#do:       "delete"
	#provider: "op"

	name:      string
	namespace: string
}

#ReadConfig: {
	#do:       "read"
	#provider: "op"

	name:      string
	namespace: string

	config: {...}
}

#ListConfig: {
	#do:       "list"
	#provider: "op"

	// Must query with the template
	template:  string
	namespace: string

	configs: [...{...}]
}

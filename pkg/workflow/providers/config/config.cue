// config.cue

#CreateConfig: {
	#do:       "create"
	#provider: "config"

	$params: {
		name:      string
		namespace: string
		template?: string
		config: {
			...
		}
	}
}

#DeleteConfig: {
	#do:       "delete"
	#provider: "config"

	$params: {
		name:      string
		namespace: string
	}
}

#ReadConfig: {
	#do:       "read"
	#provider: "config"

	$params: {
		name:      string
		namespace: string
	}

	$returns: {
		config: {...}
	}
}

#ListConfig: {
	#do:       "list"
	#provider: "config"

	$params: {
		// Must query with the template
		template:  string
		namespace: string
	}

	$returns: {
		configs: [...{...}]
	}
}

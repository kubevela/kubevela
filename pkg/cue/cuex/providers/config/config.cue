package config

#ImageRegistry: {
	#do:       "image-registry"
	#provider: "config"

	// +usage=The params of this action
	$params: {
		// +usage=Image registry FQDN, such as: index.docker.io
		registry: *"index.docker.io" | string
		// +usage=Authenticate the image registry
		auth?: {
			// +usage=Private Image registry username
			username: string
			// +usage=Private Image registry password
			password: string
			// +usage=Private Image registry email
			email?: string
		}
		// +usage=For the registry server that uses the self-signed certificate
		insecure?: bool
		// +usage=For the registry server that uses the HTTP protocol
		useHTTP?: bool
	}
	// +usage=The result of this action, will be filled with the validation response after the action is executed
	$returns: {
		// +usage=The result of the response
		result: bool
		// +usage=The message of the response
		message: string
		...
	}
	...
}

#HelmRepository: {
	#do:       "helm-repository"
	#provider: "config"

	$params: {
		// +usage=The public url of the helm chart repository.
		url: string
		// +usage=The username of basic auth repo.
		username?: string
		// +usage=The password of basic auth repo.
		password?: string
		// +usage=The ca certificate of helm repository. Please encode this data with base64.
		caFile?: string
	}
	// +usage=The result of this action, will be filled with the validation response after the action is executed
	$returns: {
		// +usage=The result of the response
		result: bool
		// +usage=The message of the response
		message: string
		...
	}
	...
}

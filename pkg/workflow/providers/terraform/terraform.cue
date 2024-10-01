// terraform.cue
#LoadTerraformComponents: {
	#provider: "terraform"
	#do:       "load-terraform-components"

	$returns: {
		outputs: {
			components: [...#Component]
		}
	}
}

#GetConnectionStatus: {
	#provider: "terraform"
	#do:       "get-connection-status"

	$params: {
		inputs: {
			componentName: string
		}
	}

	$returns: {
		outputs: {
			healthy?: bool
		}
	}
}

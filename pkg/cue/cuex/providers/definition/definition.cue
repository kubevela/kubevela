package definition

#RenderComponent: {
	#do:				"RenderComponent"
	#provider: 	"def"

	$params: {
		name?: string
		definition: string
		properties: {...}
		traits: {
			type: string
			properties: {...}
		}
	}

	$returns?: {
		output: {...}
		outputs: {...}
		...
	}
	...
}
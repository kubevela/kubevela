package definition

#RenderComponent: {
	#do:				"RenderComponent"
	#provider: 	"def"

	$params: {
		definition: string
		properties: {...}
	}

	$returns?: {
		output: {...}
		outputs: {...}
		...
	}
	...
}
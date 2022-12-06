parameter: {
	volumes: [...{
		name: string
		type: *"configMap" | "secret" | "emptyDir" | "ephemeral"
		if type == "configMap" {
			//+usage=only works when type equals configmap
			defaultMode: *420 | int
		}},
	]
}

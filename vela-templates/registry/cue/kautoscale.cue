import "encoding/json"

patch: {
	metadata: annotations: {
		"my.autoscale.ann": json.Marshal({
			"minReplicas": parameter.min
			"maxReplicas": parameter.max
		})
	}
}
parameter: {
	min: *1 | int
	max: *3 | int
}

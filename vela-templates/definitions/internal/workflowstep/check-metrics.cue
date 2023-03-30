import (
	"vela/op"
)

"check-metrics": {
	type: "workflow-step"
	labels: {
		"catalog": "Delivery"
	}
	annotations: {
		"category": "Application Delivery"
	}
	description: "Verify application's metrics"
}
template: {
	check: op.#PromCheck & {
		query:          parameter.query
		metricEndpoint: parameter.metricEndpoint
		condition:      parameter.condition
		stepID:         context.stepSessionID
		duration:       parameter.duration
		failDuration:   parameter.failDuration
	}

	fail: op.#Steps & {
		if check.failed != _|_ {
			if check.failed == true {
				breakWorkflow: op.#Fail & {
					message: check.message
				}
			}
		}
	}

	wait: op.#ConditionalWait & {
		continue: check.result
		if check.message != _|_ {
			message: check.message
		}
	}

	parameter: {
		// +usage=Query is a raw prometheus query to perform
		query: string
		// +usage=The HTTP address and port of the prometheus server
		metricEndpoint?: "http://prometheus-server.o11y-system.svc:9090" | string
		// +usage=Condition is an expression which determines if a measurement is considered successful. eg: >=0.95
		condition: string
		// +usage=Duration defines the duration of time required for this step to be considered successful.
		duration?: *"5m" | string
		// +usage=FailDuration is the duration of time that, if the check fails, will result in the step being marked as failed.
		failDuration?: *"2m" | string
	}
}

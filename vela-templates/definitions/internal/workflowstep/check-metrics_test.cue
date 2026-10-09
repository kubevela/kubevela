import "vela/test"

// _prom answers the Prometheus check, which reaches outside the cluster.
_prom: {
	_returns: {...}
	mocks: "vela/metrics": "#PromCheck": $returns: _returns
}

_query: {
	query:     "sum(rate(http_requests_total{code=\"200\"}[1m]))"
	condition: ">=0.95"
}

// _full also overrides each defaulted parameter.
_full: _query & {
	metricEndpoint: "http://prom.monitoring:9090"
	duration:       "10m"
	failDuration:   "1m"
}

"succeeds when the metric meets the condition": test.#WorkflowStepExec & _prom & {
	_returns: {result: true, failed: false}
	definition: "check-metrics"
	parameter:  _query
	expect: {
		phase: "succeeded"
		calls: "vela/metrics": "#PromCheck": [{$params: {
			query:          _query.query
			condition:      ">=0.95"
			metricEndpoint: "http://prometheus-server.o11y-system.svc:9090"
			duration:       "5m"
			failDuration:   "2m"
		}}]
	}
}

"checks against the given endpoint and durations": test.#WorkflowStepExec & _prom & {
	_returns: {result: true, failed: false}
	definition: "check-metrics"
	parameter:  _full
	expect: {
		phase: "succeeded"
		calls: "vela/metrics": "#PromCheck": [{$params: {
			query:          _query.query
			condition:      ">=0.95"
			metricEndpoint: "http://prom.monitoring:9090"
			duration:       "10m"
			failDuration:   "1m"
		}}]
	}
}

"waits while the metric has not met the condition for long enough": test.#WorkflowStepExec & _prom & {
	_returns: {result: false, failed: false, message: "The query result is 0.9, not meeting the condition"}
	definition: "check-metrics"
	parameter:  _full
	expect: {
		phase:   "running"
		message: "The query result is 0.9, not meeting the condition"
	}
}

"fails once the metric has missed the condition for the fail duration": test.#WorkflowStepExec & _prom & {
	_returns: {result: false, failed: true, message: "The query result has been 0.5 for 2m"}
	definition: "check-metrics"
	parameter:  _full
	expect: {
		phase:   "failed"
		message: "The query result has been 0.5 for 2m"
	}
}

"a query and a condition are required": test.#WorkflowStepExec & _prom & {
	_returns: {result: true, failed: false}
	definition: "check-metrics"
	parameter: {metricEndpoint: "http://prom.monitoring:9090", duration: "10m", failDuration: "1m"}
	expect: {
		phase:   "failed"
		message: =~"query: cannot convert non-concrete value"
		calls: "vela/metrics"?: _|_
	}
}

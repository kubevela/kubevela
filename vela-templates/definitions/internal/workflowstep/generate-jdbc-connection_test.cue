import "vela/test"

// The connection Secret an alibaba-rds component writes.
_connection: {
	_name:      *"rds-conn" | string
	apiVersion: "v1"
	kind:       "Secret"
	metadata: name: _name
	data: {
		DB_HOST:     "ZGIuZXhhbXBsZS5jb20=" // db.example.com
		DB_PORT:     "MzMwNg=="             // 3306
		DB_NAME:     "c2hvcA=="             // shop
		DB_USER:     "YWRtaW4="             // admin
		DB_PASSWORD: "czNjcmV0"             // s3cret
	}
}

"decodes the connection details from the Secret": test.#WorkflowStepExec & {
	definition: "generate-jdbc-connection"
	context: namespace: "rds"
	parameter: {name: "rds-conn", namespace: "rds"}
	resources: [_connection]
	expect: {
		phase: "succeeded"
		calls: {
			"vela/kube": "#Read": [{$params: value: {kind: "Secret", metadata: {name: "rds-conn", namespace: "rds"}}}]
			"vela/util": {
				"#ConvertString": [
					{$returns: str: "db.example.com"},
					{$returns: str: "3306"},
					{$returns: str: "shop"},
					{$returns: str: "admin"},
					{$returns: str: "s3cret"},
				] @contains()
			}
		}
	}
}

"reads the Secret from default when no namespace is given": test.#WorkflowStepExec & {
	definition: "generate-jdbc-connection"
	parameter: name: "rds-conn-default"
	resources: [_connection & {_name: "rds-conn-default", metadata: namespace: "default"}]
	expect: {
		phase: "succeeded"
		calls: "vela/kube": "#Read": [{$params: value: metadata: {name: "rds-conn-default", namespace?: _|_}}]
	}
}

"fails when a connection detail is missing": test.#WorkflowStepExec & {
	definition: "generate-jdbc-connection"
	context: namespace: "rds-partial"
	parameter: {name: "rds-conn", namespace: "rds-partial"}
	resources: [{apiVersion: "v1", kind: "Secret", metadata: name: "rds-conn", data: DB_HOST: "ZGIuZXhhbXBsZS5jb20="}]
	expect: {
		phase:   "failed"
		message: =~"DB_PORT"
	}
}

"fails when the Secret does not exist": test.#WorkflowStepExec & {
	definition: "generate-jdbc-connection"
	parameter: {name: "missing", namespace: "default"}
	expect: {
		phase:   "failed"
		message: =~"DB_HOST"
	}
}

"the Secret name is required": test.#WorkflowStepExec & {
	definition: "generate-jdbc-connection"
	expect: {
		phase:   "failed"
		message: =~"parameter.name"
	}
}

import (
	"vela/ql"
)

collectLogs: ql.#CollectLogsInPod & {
  cluster: "remote3"
  namespace: "default"
  pod: "express-server-f8d45d8d9-2xcnt"
  options: {
	container: "express-server"
  }
}

status: collectLogs.outputs


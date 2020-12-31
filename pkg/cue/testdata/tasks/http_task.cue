parameter: {
  serviceURL: *"http://www.baidu.com" | string
}

processing: {
  output: {
    token ?: string
  }
  task: {
    method: *"GET" | string
    url: parameter.serviceURL
    request: {
        body ?: bytes
        header: {}
        trailer: {}
    }
  }
}

patch: {
  data: token: processing.output.token
}

output: {
  data: processing.output.token
}
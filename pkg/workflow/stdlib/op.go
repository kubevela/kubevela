package stdlib

type pkg struct {
	path string
	content string
}

var opPkg=pkg{
	path: "vela/op",
	content: `

#Load: {
  #do: "load"
  #component?: string
}  

#Export: {
  #do: "export"
  type: *"schema" | "var"
  if type == "schema" {
     component: string
  }
  if type == "var"{
     path: string
  }
  value: _
}

#ConditionalWait: {
  #do: wait
  continue: bool
}

#Apply: {
  #do: "apply"
  #provider: "kube"
}

#Read: {
  #do: "read"
  #provider: "kube"
}

`,
}
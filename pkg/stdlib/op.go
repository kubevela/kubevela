package stdlib

type file struct {
	name string
	path    string
	content string
}

var opFile = file{
	name: "op.cue",
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
  #do: "wait"
  continue: bool
}

#Break: {
  #do: "break"
  message: string
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

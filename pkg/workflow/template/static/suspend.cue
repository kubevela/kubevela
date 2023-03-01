import (
	"vela/op"
)

suspend: op.#Suspend & {
	if parameter.duration != _|_ {
		duration: parameter.duration
	}
	if parameter.message != _|_ {
		message: parameter.message
	}
}

parameter: {
	duration?: string
	message?:  string
}

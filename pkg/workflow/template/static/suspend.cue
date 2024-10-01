import (
	"vela/builtin"
)

suspend: builtin.#Suspend & {
	$params: parameter
}

parameter: {
	duration?: string
	message?:  string
}

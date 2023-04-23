import "strings"

label: "app"
// alias for label
let L = label
S: {
	name: "Postgres"
	// intermediate value
	let lower = strings.ToLower(name)
	version: "13"
	label:   L
	image:   "docker.io/\(lower):\(version)"
}

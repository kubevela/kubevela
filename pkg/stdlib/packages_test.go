package stdlib

import (
	"testing"

	"github.com/bmizerany/assert"
)

func TestGetPackages(t *testing.T) {
	d := &discover{}
	f1 := file{
		name: "network.cue",
		path: "kube/core",
		content: `
service: {
    apiVersion: "v1"
    kind: "Service"
}
`,
	}

	f2 := file{
		name: "security.cue",
		path: "kube/core",
		content: `
secret: {
    apiVersion: "v1"
    kind: "Secret"
}
`,
	}

	f3 := file{
		name: "apps.cue",
		path: "kube/apps",
		content: `
deployment: {
    apiVersion: "apps/v1"
    kind: "Deployment"
}
`,
	}
	d.addFile(f1)
	d.addFile(f2)
	d.addFile(f3)
	for path, pkg := range d.packages() {
		switch path {
		case "kube/core":
			assert.Equal(t, pkg, `
service: {
    apiVersion: "v1"
    kind: "Service"
}


secret: {
    apiVersion: "v1"
    kind: "Secret"
}

`)
		case "kube/apps":
			assert.Equal(t, pkg, `
deployment: {
    apiVersion: "apps/v1"
    kind: "Deployment"
}

`)
		default:
			t.Error("package path invalid")
		}

	}
}

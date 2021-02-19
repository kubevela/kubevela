package process

import (
	"testing"

	"cuelang.org/go/cue"
	"github.com/bmizerany/assert"

	"github.com/oam-dev/kubevela/pkg/dsl/model"
)

func TestContext(t *testing.T) {
	baseTemplate := `
image: "myserver"
`

	var r cue.Runtime
	inst, err := r.Compile("-", baseTemplate)
	if err != nil {
		t.Error(err)
		return
	}
	base, err := model.NewBase(inst.Value())
	if err != nil {
		t.Error(err)
		return
	}

	ctx := NewContext("mycomp", "myapp")
	ctx.SetBase(base)
	ctxInst, err := r.Compile("-", ctx.BaseContextFile())
	if err != nil {
		t.Error(err)
		return
	}

	gName, err := ctxInst.Lookup("context", "name").String()
	assert.Equal(t, nil, err)
	assert.Equal(t, "mycomp", gName)

	myAppName, err := ctxInst.Lookup("context", "appName").String()
	assert.Equal(t, nil, err)
	assert.Equal(t, "myapp", myAppName)
	inputJs, err := ctxInst.Lookup("context", "output").MarshalJSON()
	assert.Equal(t, nil, err)
	assert.Equal(t, `{"image":"myserver"}`, string(inputJs))
}

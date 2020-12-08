package processer

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

	ctx := NewContext("myctx")
	ctx.SetBase(base)
	ctxInst, err := r.Compile("-", ctx.Compile("context"))
	if err != nil {
		t.Error(err)
		return
	}

	gName, err := ctxInst.Lookup("context", "name").String()
	assert.Equal(t, nil, err)
	assert.Equal(t, "myctx", gName)
	inputJs, err := ctxInst.Lookup("context", "input").MarshalJSON()
	assert.Equal(t, nil, err)
	assert.Equal(t, `{"image":"myserver"}`, string(inputJs))
}

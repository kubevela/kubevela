package definitions_tests

import (
	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"fmt"
	"github.com/stretchr/testify/assert"
	"os"
	"testing"
)

func TestTraits(t *testing.T) {
	cuectx := cuecontext.New()
	// parse input
	inputBytes, err := os.ReadFile("./expose/input.cue")
	assert.Nil(t, err)
	traitBytes, err := os.ReadFile("./expose/trait.cue")
	assert.Nil(t, err)

	v := cuectx.CompileBytes(append(traitBytes, inputBytes...))

	outputs := v.LookupPath(cue.ParsePath("template.outputs"))
	assert.True(t, outputs.Exists())

	jsonv, err := outputs.MarshalJSON()
	fmt.Println(string(jsonv))

	// parse output

	outputBytes, err := os.ReadFile("./expose/output.cue")
	v = cuectx.CompileBytes(append(outputBytes, inputBytes...))
	outputs = v.LookupPath(cue.ParsePath("template.outputs"))
	outputJsonv, err := outputs.MarshalJSON()
	fmt.Println(string(outputJsonv))

	assert.JSONEq(t, string(outputJsonv), string(jsonv))
}

package model

import (
	"testing"

	"cuelang.org/go/cue"
	"github.com/bmizerany/assert"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestInstance(t *testing.T) {

	testCases := []struct {
		src string
		gvk schema.GroupVersionKind
	}{{
		src: `apiVersion: "apps/v1"
kind: "Deployment"
metadata: name: "test"
`,
		gvk: schema.GroupVersionKind{
			Group:   "apps",
			Version: "v1",
			Kind:    "Deployment",
		}},
	}

	for _, v := range testCases {
		var r cue.Runtime
		inst, err := r.Compile("-", v.src)
		if err != nil {
			t.Error(err)
			return
		}
		base, err := NewBase(inst.Value())
		if err != nil {
			t.Error(err)
			return
		}
		baseObj, err := base.Unstructured()
		if err != nil {
			t.Error(err)
			return
		}

		assert.Equal(t, v.gvk, baseObj.GetObjectKind().GroupVersionKind())
		assert.Equal(t, true, base.IsBase())

		other, err := NewOther(inst.Value())
		if err != nil {
			t.Error(err)
			return
		}
		otherObj, err := other.Unstructured()
		if err != nil {
			t.Error(err)
			return
		}

		assert.Equal(t, v.gvk, otherObj.GetObjectKind().GroupVersionKind())
		assert.Equal(t, false, other.IsBase())
	}
}

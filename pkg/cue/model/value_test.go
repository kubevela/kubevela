package model

import (
	"fmt"
	"github.com/oam-dev/kubevela/pkg/cue/model/sets"
	"testing"
)

func TestFillObject(t *testing.T) {
	var src = `
   ops: {
   x: "123"
   x1: #tt
   up: {
     apply: int
     uu: apply
   }
   x2: #tt
   out: up["apply"]
}
  #tt: {}


iter:{

step1: {}
step2: {prefix: step1.value}
if step2.value>3{
step2_3: {}
}
step3: {prefix: step2.value}
step4: {prefix: step3.value}
if step4.value>100{
 step5: "xxxx"
}
}
`

	var js = `
  {"apply": 123}
`

	sv, _ := NewValue(src,nil)
	up, _ := sv.MakeValue(js)
	fmt.Println(up.String())
	up.FillRaw(`x: 12345`, "to")
	fmt.Println(sv.FillObject(up, "ops", "up"))
	fmt.Println(sv.String())
	fmt.Println(sv.Field("#t"))

	child, _ := sv.LookupValue("iter")
	number := 99
	child.StepFields(func(in *Value) (bool, error) {
		number++
		return false, in.FillObject(map[string]interface{}{
			"value": number,
		})
	})

	fmt.Println(sets.ToString(child.v))
}

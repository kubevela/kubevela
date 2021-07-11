package model

import (
	"fmt"
	"github.com/oam-dev/kubevela/pkg/cue/model/sets"
	"testing"
)

func TestFillObject(t *testing.T){
	var src=`
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
step3: {prefix: step2.value}
step4: {prefix: step3.value}
}
`

var js= `
  {"apply": 123}
`

sv,_:=NewValue(src)
up,_:=sv.MakeValue(js)
fmt.Println(up.String())
up.FillRaw(`x: 12345`,"to")
fmt.Println(sv.FillObject(up,"ops","up"))
fmt.Println(sv.String())
fmt.Println(sv.Filed("#t"))



	child,_:=sv.LookupValue("iter")

	child.WalkFields(func(in *Value) error {
		return in.FillRaw("{\"value\": 100}")
	})

fmt.Println(sets.ToString(child.v))
}

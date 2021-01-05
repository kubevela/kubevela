package definition

import (
	"encoding/json"
	"fmt"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/build"
	"github.com/pkg/errors"

	"github.com/oam-dev/kubevela/pkg/dsl/model"
	"github.com/oam-dev/kubevela/pkg/dsl/process"
)

// Template defines Definition's Render interface
type Template interface {
	Params(params interface{}) Template
	Complete(ctx process.Context) error
}

type def struct {
	name   string
	templ  string
	params interface{}
}

type workloadDef struct {
	def
}

// NewWDTemplater create Workload Definition templater
func NewWDTemplater(name, templ string) Template {
	return &workloadDef{
		def: def{
			name:   name,
			templ:  templ,
			params: nil,
		},
	}
}

// Params set definition's params
func (wd *workloadDef) Params(params interface{}) Template {
	wd.params = params
	return wd
}

// Complete do workload definition's rendering
func (wd *workloadDef) Complete(ctx process.Context) error {
	bi := build.NewContext().NewInstance("", nil)
	if err := bi.AddFile("-", wd.templ); err != nil {
		return err
	}
	if wd.params != nil {
		bt, _ := json.Marshal(wd.params)
		if err := bi.AddFile("parameter", fmt.Sprintf("parameter: %s", string(bt))); err != nil {
			return err
		}
	}

	if err := bi.AddFile("-", ctx.Compile("context")); err != nil {
		return err
	}
	insts := cue.Build([]*build.Instance{bi})
	for _, inst := range insts {
		if err := inst.Value().Err(); err != nil {
			return errors.WithMessagef(err, "workloadDef %s eval", wd.name)
		}
		output := inst.Lookup("output")
		base, err := model.NewBase(output)
		if err != nil {
			return errors.WithMessagef(err, "workloadDef %s new base", wd.name)
		}
		ctx.SetBase(base)
	}
	return nil
}

type traitDef struct {
	def
}

// NewTDTemplater create Trait Definition templater
func NewTDTemplater(name, templ string) Template {
	return &traitDef{
		def: def{
			name:  name,
			templ: templ,
		},
	}
}

// Params set definition's params
func (td *traitDef) Params(params interface{}) Template {
	td.params = params
	return td
}

// Complete do trait definition's rendering
func (td *traitDef) Complete(ctx process.Context) error {
	bi := build.NewContext().NewInstance("", nil)
	if err := bi.AddFile("-", td.templ); err != nil {
		return err
	}
	if td.params != nil {
		bt, _ := json.Marshal(td.params)
		if err := bi.AddFile("parameter", fmt.Sprintf("parameter: %s", string(bt))); err != nil {
			return err
		}
	}

	if err := bi.AddFile("f", ctx.Compile("context")); err != nil {
		return err
	}
	insts := cue.Build([]*build.Instance{bi})
	for _, inst := range insts {

		if err := inst.Value().Err(); err != nil {
			return errors.WithMessagef(err, "traitDef %s build", td.name)
		}

		output := inst.Lookup("output")
		if output.Exists() {
			other, err := model.NewOther(output)
			if err != nil {
				return errors.WithMessagef(err, "traitDef %s new Assist", td.name)
			}
			ctx.PutAssistants(process.Assistant{Ins: other, Type: td.name})
		}

		outputs := inst.Lookup("outputs")
		st, err := outputs.Struct()
		if err == nil {
			for i := 0; i < st.Len(); i++ {
				fieldInfo := st.Field(i)
				if fieldInfo.IsDefinition || fieldInfo.IsHidden || fieldInfo.IsOptional {
					continue
				}
				other, err := model.NewOther(fieldInfo.Value)
				if err != nil {
					return errors.WithMessagef(err, "traitDef %s new Assists(%s)", td.name, fieldInfo.Name)
				}
				ctx.PutAssistants(process.Assistant{Ins: other, Type: td.name})
			}

		}

		patcher := inst.Lookup("patch")
		if patcher.Exists() {
			base, _ := ctx.Output()
			p, err := model.NewOther(patcher)
			if err != nil {
				return errors.WithMessagef(err, "traitDef %s patcher NewOther", td.name)
			}
			if err := base.Unify(p); err != nil {
				return err
			}
		}

	}
	return nil
}

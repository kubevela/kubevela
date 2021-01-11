package process

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode"

	"github.com/oam-dev/kubevela/pkg/dsl/model"
)

// Context defines Rendering Context Interface
type Context interface {
	SetBase(base model.Instance)
	PutAssistants(insts ...Assistant)
	SetConfigs(configs []map[string]string)
	Output() (model.Instance, []Assistant)
	Compile(label string) string
}

// Assistant are objects rendered by definition template.
type Assistant struct {
	Ins  model.Instance
	Type string
}

type context struct {
	name       string
	configs    []map[string]string
	base       model.Instance
	assistants []Assistant
}

// NewContext create render context
func NewContext(name string) Context {
	return &context{
		name:       name,
		configs:    []map[string]string{},
		assistants: []Assistant{},
	}
}

// SetBase set context base model
func (ctx *context) SetConfigs(configs []map[string]string) {
	ctx.configs = configs
}

// SetBase set context base model
func (ctx *context) SetBase(base model.Instance) {
	ctx.base = base
}

// PutAssistants add Assist model to context
func (ctx *context) PutAssistants(insts ...Assistant) {
	ctx.assistants = append(ctx.assistants, insts...)
}

// Compile return cue format string of context
func (ctx *context) Compile(label string) string {
	var buff string
	buff += fmt.Sprintf("name: \"%s\"\n", ctx.name)

	if ctx.base != nil {
		buff += fmt.Sprintf("input: %s\n", structMarshal(ctx.base.String()))
	}

	if len(ctx.configs) > 0 {
		bt, _ := json.Marshal(ctx.configs)
		buff += "config: " + string(bt)
	}

	if label != "" {
		buff = fmt.Sprintf("%s: %s", label, structMarshal(buff))
	}

	return buff
}

// Output return models of context
func (ctx *context) Output() (model.Instance, []Assistant) {
	return ctx.base, ctx.assistants
}

func structMarshal(v string) string {
	skip := false
	v = strings.TrimFunc(v, func(r rune) bool {
		if !skip {
			if unicode.IsSpace(r) {
				return true
			}
			skip = true

		}
		return false
	})

	if strings.HasPrefix(v, "{") {
		return v
	}
	return fmt.Sprintf("{%s}", v)
}

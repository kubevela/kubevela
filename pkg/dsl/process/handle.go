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
	AppendAuxiliaries(auxiliaries ...Auxiliary)
	SetConfigs(configs []map[string]string)
	Output() (model.Instance, []Auxiliary)
	BaseContextFile() string
	BaseContextLabels() map[string]string
}

// Auxiliary are objects rendered by definition template.
type Auxiliary struct {
	Ins model.Instance
	// Type will be used to mark definition label for OAM runtime to get the CRD
	// It's now required for trait and main workload object. Extra workload CR object will not have the type.
	Type string

	// Workload or trait with multiple `outputs` will have a name, if name is empty, than it's the main of this type.
	Name string

	// IsOutputs will record the output path format of the Auxiliary
	// it can be one of these two cases:
	// false: the format is `output`, this means it's the main resource of the trait
	// true: the format is `outputs.<resourceName>`, this means it can be auxiliary workload or trait
	IsOutputs bool
}

type templateContext struct {
	// name is the component name of Application
	name string
	// appName is the name of Application
	appName     string
	configs     []map[string]string
	base        model.Instance
	auxiliaries []Auxiliary

	// TODO(wonderflow): add a revision number here, and it should be a suffix combined with appName to be the name of AppConfig
}

// NewContext create render templateContext
func NewContext(name, appName string) Context {
	return &templateContext{
		name:        name,
		appName:     appName,
		configs:     []map[string]string{},
		auxiliaries: []Auxiliary{},
	}
}

// SetBase set templateContext base model
func (ctx *templateContext) SetConfigs(configs []map[string]string) {
	ctx.configs = configs
}

// SetBase set templateContext base model
func (ctx *templateContext) SetBase(base model.Instance) {
	ctx.base = base
}

// AppendAuxiliaries add Assist model to templateContext
func (ctx *templateContext) AppendAuxiliaries(auxiliaries ...Auxiliary) {
	ctx.auxiliaries = append(ctx.auxiliaries, auxiliaries...)
}

// BaseContextFile return cue format string of templateContext
func (ctx *templateContext) BaseContextFile() string {
	var buff string
	buff += fmt.Sprintf("name: \"%s\"\n", ctx.name)
	buff += fmt.Sprintf("appName: \"%s\"\n", ctx.appName)

	if ctx.base != nil {
		buff += fmt.Sprintf("input: %s\n", structMarshal(ctx.base.String()))
	}

	if len(ctx.configs) > 0 {
		bt, _ := json.Marshal(ctx.configs)
		buff += "config: " + string(bt)
	}

	return fmt.Sprintf("context: %s", structMarshal(buff))
}

func (ctx *templateContext) BaseContextLabels() map[string]string {

	return map[string]string{
		// appName is oam.LabelAppName
		"appName": ctx.appName,
		// name is oam.LabelAppComponent
		"name": ctx.name,
	}
}

// Output return model and auxiliaries of templateContext
func (ctx *templateContext) Output() (model.Instance, []Auxiliary) {
	return ctx.base, ctx.auxiliaries
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

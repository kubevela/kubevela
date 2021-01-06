package builtin

import (
	"strings"

	"github.com/pkg/errors"

	cmdutil "github.com/oam-dev/kubevela/pkg/commands/util"
)

var (
	tasks = map[string]Task{}
)

// Task process app-file
type Task func(ctx CallCtx, params interface{}) error

// CallCtx is task handle context
type CallCtx interface {
	LookUp(...string) (interface{}, error)
	IO() cmdutil.IOStreams
}

type callContext struct {
	data      map[string]interface{}
	ioStreams cmdutil.IOStreams
}

// IO return io streams handler
func (ctx *callContext) IO() cmdutil.IOStreams {
	return ctx.ioStreams
}

// LookUp find value by paths
func (ctx *callContext) LookUp(paths ...string) (interface{}, error) {
	var walkData interface{} = ctx.data

	for _, path := range paths {
		walkData = lookup(walkData, path)
		if walkData == nil {
			return nil, errors.Errorf("lookup field '%s' : not found", strings.Join(paths, "."))
		}
	}
	return walkData, nil
}

func lookup(v interface{}, key string) interface{} {
	val, ok := v.(map[string]interface{})
	if ok {
		return val[key]
	}
	return nil
}

func newCallCtx(io cmdutil.IOStreams, data map[string]interface{}) CallCtx {
	return &callContext{
		ioStreams: io,
		data:      data,
	}
}

// RegisterTask register task for appfile
func RegisterTask(name string, task Task) {
	tasks[name] = task
}

// DoTasks do init tasks for app-file
func DoTasks(spec map[string]interface{}, io cmdutil.IOStreams) (map[string]interface{}, error) {
	var (
		ctx     = newCallCtx(io, spec)
		retSpec = map[string]interface{}{}
	)

	for key, params := range spec {
		if do, ok := tasks[key]; ok {
			if err := do(ctx, params); err != nil {
				return nil, errors.WithMessagef(err, "do task %s", key)
			}
		} else {
			retSpec[key] = params
		}
	}
	return retSpec, nil
}

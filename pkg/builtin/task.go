package builtin

import (
	"errors"

	"cuelang.org/go/cue"

	// RegisterRunner all build jobs here, so the jobs will automatically registered before RunBuildInTasks run.
	_ "github.com/oam-dev/kubevela/pkg/builtin/build"
	_ "github.com/oam-dev/kubevela/pkg/builtin/http"

	"github.com/oam-dev/kubevela/pkg/builtin/registry"
	cmdutil "github.com/oam-dev/kubevela/pkg/commands/util"
)

// RunBuildInTasks do initializing tasks for appfile.
// You should call RunBuildInTasks instead of directly call registry.Run as the package will automatically import internal packages for built-in task
func RunBuildInTasks(spec map[string]interface{}, io cmdutil.IOStreams) (map[string]interface{}, error) {
	return registry.Run(spec, io)
}

// RunTaskByKey do task by key
func RunTaskByKey(key string, v cue.Value, meta *registry.Meta) (interface{}, error) {
	task := registry.LookupRunner(key)
	if task == nil {
		return nil, errors.New("there is no http task in task registry")
	}
	runner, err := task(v)
	if err != nil {
		return nil, err
	}
	return runner.Run(meta)
}

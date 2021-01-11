package builtin

import (
	// Register all build jobs here, so the jobs will automatically registered before RunBuildInTasks run.
	_ "github.com/oam-dev/kubevela/pkg/builtin/build"

	"github.com/oam-dev/kubevela/pkg/builtin/registry"
	cmdutil "github.com/oam-dev/kubevela/pkg/commands/util"
)

// RunBuildInTasks do initializing tasks for appfile.
// You should call RunBuildInTasks instead of directly call registry.Run as the package will automatically import internal packages for built-in task
func RunBuildInTasks(spec map[string]interface{}, io cmdutil.IOStreams) (map[string]interface{}, error) {
	return registry.Run(spec, io)
}

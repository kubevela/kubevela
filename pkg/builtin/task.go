package builtin

import (
	// Register all build jobs here, so the jobs will automatically registered before DoTasks run.
	_ "github.com/oam-dev/kubevela/pkg/builtin/build"

	"github.com/oam-dev/kubevela/pkg/builtin/registry"
	cmdutil "github.com/oam-dev/kubevela/pkg/commands/util"
)

// DoTasks do initializing tasks for appfile.
// You should call DoTasks instead of directly call registry.Run as the package will automatically import internal packages for built-in task
func DoTasks(spec map[string]interface{}, io cmdutil.IOStreams) (map[string]interface{}, error) {
	return registry.Run(spec, io)
}

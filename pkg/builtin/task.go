/*
Copyright 2021 The KubeVela Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package builtin

import (
	"errors"

	"cuelang.org/go/cue"

	// RegisterRunner all build jobs here, so the jobs will automatically registered before RunBuildInTasks run.
	_ "github.com/oam-dev/kubevela/pkg/builtin/build"
	_ "github.com/oam-dev/kubevela/pkg/builtin/http"

	"github.com/oam-dev/kubevela/pkg/builtin/registry"
	cmdutil "github.com/oam-dev/kubevela/pkg/utils/util"
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

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

package main

import (
	"context"
	"fmt"
	"os"

	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	"github.com/oam-dev/kubevela/references/plugins"
)

func main() {
	ref := &plugins.MarkdownReference{}
	ctx := context.Background()
	path := plugins.BaseRefPath

	if len(os.Args) == 2 {
		ref.DefinitionName = os.Args[1]
		path = plugins.KubeVelaIOTerraformPath
	}

	c, err := common.InitBaseRestConfig()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	if err := ref.GenerateReferenceDocs(ctx, c, path, types.DefaultKubeVelaNS); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

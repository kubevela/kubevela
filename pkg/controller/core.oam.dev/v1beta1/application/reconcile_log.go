/*
Copyright 2026 The KubeVela Authors.

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

package application

import (
	monitorContext "github.com/kubevela/pkg/monitor/context"
	"k8s.io/klog/v2"

	common2 "github.com/oam-dev/kubevela/pkg/controller/common"
)

func debugLogEnabled() bool {
	return klog.V(common2.LogDebug).Enabled()
}

func logDebug(ctx monitorContext.Context, msg string, keysAndValues ...interface{}) {
	if debugLogEnabled() {
		ctx.InfoDepth(1, msg, keysAndValues...)
	}
}

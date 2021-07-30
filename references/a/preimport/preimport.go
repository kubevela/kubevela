/*
 Copyright 2021. The KubeVela Authors.

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

package preimport

import (
	"flag"

	"github.com/containerd/containerd/log"
	"github.com/sirupsen/logrus"
	"k8s.io/klog"
)

var _flagSet *flag.FlagSet

// disable logging during import
func init() {

	log.L.Logger.SetLevel(logrus.PanicLevel)
	_flagSet = flag.CommandLine
	klog.InitFlags(_flagSet)
	_ = _flagSet.Set("v", "-1")
}

// Start recover logging
func Start() {

	log.L.Logger.SetLevel(logrus.InfoLevel)
	_ = _flagSet.Set("v", "0")
}

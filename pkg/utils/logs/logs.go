/*
 Copyright 2014 The Kubernetes Authors.

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

package logs

import (
	"flag"
	"io/ioutil"
	"log"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog"
)

func init() {
	klog.InitFlags(flag.CommandLine)
	_ = flag.Set("logtostderr", "true")
}

// KlogWriter serves as a bridge between the standard log package and the glog package.
type KlogWriter struct{}

// Write implements the io.Writer interface.
func (writer KlogWriter) Write(data []byte) (n int, err error) {
	klog.InfoDepth(1, string(data))
	return len(data), nil
}

// InitLogs initializes logs the way we want for kubernetes.
func InitLogs() {
	log.SetOutput(ioutil.Discard)
	klog.SetOutput(ioutil.Discard)
	log.SetFlags(-1)

	// The default glog flush interval is one second.
	go wait.Until(klog.Flush, time.Second, wait.NeverStop)
}

// FlushLogs flushes logs immediately.
func FlushLogs() {
	klog.Flush()
}

// NewLogger creates a new log.Logger which sends logs to klog.Info.
func NewLogger(prefix string) *log.Logger {
	return log.New(KlogWriter{}, prefix, 0)
}

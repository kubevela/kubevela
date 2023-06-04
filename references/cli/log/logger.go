/*
Copyright 2023 The KubeVela Authors.

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

package log

import (
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/go-logr/logr"
	"github.com/kubevela/pkg/util/slices"
	"github.com/kubevela/pkg/util/stringtools"
	"k8s.io/klog/v2"
)

var _ logr.LogSink = &logger{}

// NewLogger create a logr.Logger with vela-cli format
func NewLogger(name string) logr.Logger {
	return NewLoggerWithWriter(name, os.Stdout, os.Stderr)
}

// NewLoggerWithWriter create a logr.Logger with vela-cli format and customized writer
func NewLoggerWithWriter(name string, out io.Writer, errWriter io.Writer) logr.Logger {
	return logr.New(&logger{name: name, out: out, errWriter: errWriter})
}

type logger struct {
	name      string
	callDepth int
	values    []interface{}
	out       io.Writer
	errWriter io.Writer
}

// Init init
func (in *logger) Init(info logr.RuntimeInfo) {
	in.callDepth += info.CallDepth
}

// Enabled check if enabled
func (in *logger) Enabled(level int) bool {
	return klog.VDepth(in.callDepth+2, klog.Level(level)).Enabled()
}

func (in *logger) mergeKeysAndValues(keysAndValues []interface{}) string {
	lookup := map[string]string{}
	var keys []string
	for i := 0; i < len(keysAndValues); i += 2 {
		key := fmt.Sprintf("%s", keysAndValues[i])
		value := fmt.Sprintf("%s", keysAndValues[i+1])
		if _, found := lookup[key]; !found {
			keys = append(keys, key)
		}
		lookup[key] = fmt.Sprintf("%s=\"%s\"", key, value)
	}
	return strings.Join(slices.Map(keys, func(key string) string { return lookup[key] }), " ")
}

func (in *logger) print(writer io.Writer, head string, msg string, keysAndValues ...interface{}) {
	var caller, timeStr, nameStr string
	if klog.V(4).Enabled() {
		nameStr = color.MagentaString(fmt.Sprintf("%s ", in.name))
		if _, file, line, ok := runtime.Caller(7); ok {
			if !klog.V(7).Enabled() {
				file = file[strings.LastIndex(file, "/")+1:]
			}
			caller = fmt.Sprintf("[%s:%d] ", file, line)
		}
		t := time.Now()
		timeStr = fmt.Sprintf("%02d:%02d:%02d ", t.Hour(), t.Minute(), t.Second())
		if klog.V(7).Enabled() {
			timeStr = t.Format(time.RFC3339) + " "
		}
	}
	_, _ = fmt.Fprintf(writer, "%s %s%s%s%s %s\n", head, nameStr, timeStr, caller, strings.TrimSpace(stringtools.Capitalize(msg)), in.mergeKeysAndValues(keysAndValues))
}

// Info .
func (in *logger) Info(level int, msg string, keysAndValues ...interface{}) {
	if level < 1 {
		level = 1
	}
	if in.Enabled(level) {
		in.print(in.out, color.CyanString("INFO"), msg, append(in.values, keysAndValues...)...)
	}
}

// Error .
func (in *logger) Error(err error, msg string, keysAndValues ...interface{}) {
	if err != nil {
		keysAndValues = append(keysAndValues, "err", err.Error())
	}
	in.print(in.errWriter, color.RedString("ERR "), msg, append(in.values, keysAndValues...)...)
}

// WithValues fork a logger with given key-values
func (in *logger) WithValues(keysAndValues ...interface{}) logr.LogSink {
	sink := &logger{}
	*sink = *in
	if len(keysAndValues)%2 != 0 {
		keysAndValues = append(keysAndValues, "(MISSING)")
	}
	sink.values = append(in.values, keysAndValues...) // nolint
	return sink
}

// WithName fork a logger with given name
func (in *logger) WithName(name string) logr.LogSink {
	sink := &logger{}
	*sink = *in
	sink.name = name
	return sink
}

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

package util

import (
	"bytes"
	"fmt"
	"io"
	"os"
)

// IOStreams provides the standard names for iostreams.  This is useful for embedding and for unit testing.
// Inconsistent and different names make it hard to read and review code
type IOStreams struct {
	// In think, os.Stdin
	In io.Reader
	// Out think, os.Stdout
	Out io.Writer
	// ErrOut think, os.Stderr
	ErrOut io.Writer
}

// Infonln compared to Info(), won't print new line
func (i *IOStreams) Infonln(a ...interface{}) {
	_, _ = fmt.Fprint(i.Out, a...)
}

// Info print info with new line
func (i *IOStreams) Info(a ...interface{}) {
	_, _ = fmt.Fprintln(i.Out, a...)
}

// Infof print info in a specified format
func (i *IOStreams) Infof(format string, a ...interface{}) {
	_, _ = fmt.Fprintf(i.Out, format, a...)
}

// Errorf print error info in a specified format
func (i *IOStreams) Errorf(format string, a ...interface{}) {
	_, _ = fmt.Fprintf(i.ErrOut, format, a...)
}

// Error print error info
func (i *IOStreams) Error(a ...interface{}) {
	_, _ = fmt.Fprintln(i.ErrOut, a...)
}

// NewDefaultIOStreams return IOStreams with standard input/output/error
func NewDefaultIOStreams() IOStreams {
	return IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr}
}

// NewTestIOStreams return IOStreams with empty input and combined buffered output
func NewTestIOStreams() (IOStreams, *bytes.Buffer) {
	var buf bytes.Buffer
	return IOStreams{In: &bytes.Buffer{}, Out: &buf, ErrOut: &buf}, &buf
}

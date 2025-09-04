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
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIOStreams_PrintFunctions(t *testing.T) {
	type printFunc func(streams IOStreams)
	type checkFunc func(t *testing.T, buf *bytes.Buffer)

	testCases := []struct {
		name      string
		printFunc printFunc
		checkFunc checkFunc
	}{
		{
			name: "Infonln",
			printFunc: func(streams IOStreams) {
				streams.Infonln("hello", " ", "world")
			},
			checkFunc: func(t *testing.T, buf *bytes.Buffer) {
				assert.Equal(t, "hello world", buf.String())
			},
		},
		{
			name: "Info",
			printFunc: func(streams IOStreams) {
				streams.Info("hello")
			},
			checkFunc: func(t *testing.T, buf *bytes.Buffer) {
				assert.Equal(t, "hello\n", buf.String())
			},
		},
		{
			name: "Infof",
			printFunc: func(streams IOStreams) {
				streams.Infof("hello %s", "world")
			},
			checkFunc: func(t *testing.T, buf *bytes.Buffer) {
				assert.Equal(t, "hello world", buf.String())
			},
		},
		{
			name: "Error",
			printFunc: func(streams IOStreams) {
				streams.Error("error")
			},
			checkFunc: func(t *testing.T, buf *bytes.Buffer) {
				assert.Equal(t, "error\n", buf.String())
			},
		},
		{
			name: "Errorf",
			printFunc: func(streams IOStreams) {
				streams.Errorf("error %s", "world")
			},
			checkFunc: func(t *testing.T, buf *bytes.Buffer) {
				assert.Equal(t, "error world", buf.String())
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			streams, buf := NewTestIOStreams()
			tc.printFunc(streams)
			tc.checkFunc(t, buf)
		})
	}
}

func TestNewDefaultIOStreams(t *testing.T) {
	streams := NewDefaultIOStreams()
	assert.Equal(t, os.Stdin, streams.In)
	assert.Equal(t, os.Stdout, streams.Out)
	assert.Equal(t, os.Stderr, streams.ErrOut)
}

func TestNewTestIOStreams(t *testing.T) {
	streams, buf := NewTestIOStreams()
	assert.NotNil(t, streams.In)
	assert.IsType(t, &bytes.Buffer{}, streams.In)
	assert.Equal(t, streams.Out, buf)
	assert.Equal(t, streams.ErrOut, buf)
}

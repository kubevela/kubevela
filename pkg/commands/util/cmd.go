package util

import (
	"fmt"
	"io"
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
	_, _ = i.Out.Write([]byte(fmt.Sprint(a...)))
}

// Info print info with new line
func (i *IOStreams) Info(a ...interface{}) {
	_, _ = i.Out.Write([]byte(fmt.Sprintln(a...)))
}

// Infof print info in a specified format
func (i *IOStreams) Infof(format string, a ...interface{}) {
	_, _ = i.Out.Write([]byte(fmt.Sprintf(format, a...)))
}

// Errorf print error info in a specified format
func (i *IOStreams) Errorf(format string, a ...interface{}) {
	_, _ = i.ErrOut.Write([]byte(fmt.Sprintf(format, a...)))
}

// Error print error info
func (i *IOStreams) Error(a ...interface{}) {
	_, _ = i.ErrOut.Write([]byte(fmt.Sprintln(a...)))
}

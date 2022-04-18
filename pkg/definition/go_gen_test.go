package definition

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefaultFieldNamer(t *testing.T) {
	for name, expected := range map[string]string{
		"id":         "ID",
		"foo":        "Foo",
		"foo_bar":    "FooBar",
		"fooBar":     "FooBar",
		"FOO_BAR":    "FooBar",
		"FOO_BAR_ID": "FooBarID",
		"123":        "_123",
		"A|B":        "A_B",
	} {
		assert.Equal(t, expected, dm.FieldName(name))
	}
}

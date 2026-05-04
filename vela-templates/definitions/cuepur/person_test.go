package example_test

import (
	"os"
	"testing"

	"cuelang.org/go/cue/cuecontext"
	"golang.org/x/tools/txtar"
)

func TestPerson(t *testing.T) {
	// Read the txtar test file
	txtarData, err := os.ReadFile("testdata/person.txtar")
	if err != nil {
		t.Fatalf("failed to read testdata/person.txtar: %v", err)
	}

	// Parse the txtar archive
	archive := txtar.Parse(txtarData)

	// Build a map of files from the archive
	files := make(map[string]string)
	for _, file := range archive.Files {
		files[file.Name] = string(file.Data)
	}

	// Verify we have the required files
	if _, ok := files["in.cue"]; !ok {
		t.Fatal("txtar missing in.cue file")
	}
	if _, ok := files["out.cue"]; !ok {
		t.Fatal("txtar missing out.cue file")
	}

	ctx := cuecontext.New()

	// Compile the input CUE
	actual := ctx.CompileString(files["in.cue"])
	if actual.Err() != nil {
		t.Fatalf("failed to compile in.cue: %v", actual.Err())
	}

	// Compile the expected output
	expected := ctx.CompileString(files["out.cue"])
	if expected.Err() != nil {
		t.Fatalf("failed to compile out.cue: %v", expected.Err())
	}

	// Validate that actual output matches expected output using CUE unification
	// The expected output should subsume the actual output (be more general or equal)
	unified := expected.Unify(actual)
	if unified.Err() != nil {
		t.Errorf("actual output does not match expected output: %v", unified.Err())

		// Print detailed diff for debugging
		actualJSON, _ := actual.MarshalJSON()
		expectedJSON, _ := expected.MarshalJSON()
		t.Logf("Actual output:\n%s", string(actualJSON))
		t.Logf("Expected output:\n%s", string(expectedJSON))
	}

	// Validate the unified result is concrete (no errors)
	if err := unified.Validate(); err != nil {
		t.Errorf("unified result validation failed: %v", err)
	}
}

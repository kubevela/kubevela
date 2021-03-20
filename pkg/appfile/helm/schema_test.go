package helm

import (
	"encoding/json"
	"io/ioutil"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestGenerateSchemaFromValues(t *testing.T) {
	testValues, err := ioutil.ReadFile("./testdata/values.yaml")
	if err != nil {
		t.Error(err, "cannot load test data")
	}
	wantSchema, err := ioutil.ReadFile("./testdata/values.schema.json")
	if err != nil {
		t.Error(err, "cannot load expected data")
	}
	wantSchemaMap := map[string]interface{}{}
	// convert bytes to map for diff converience
	_ = json.Unmarshal(wantSchema, &wantSchemaMap)
	result, err := generateSchemaFromValues(testValues)
	if err != nil {
		t.Error(err, "failed generate schema from values")
	}
	resultMap := map[string]interface{}{}
	if err := json.Unmarshal(result, &resultMap); err != nil {
		t.Error(err, "cannot unmarshal result bytes")
	}
	if diff := cmp.Diff(resultMap, wantSchemaMap); diff != "" {
		t.Fatalf("generateSchemaFromValues(...) -want +get \n%s", diff)
	}
}

package serverlib

import (
	"encoding/json"
	"github.com/oam-dev/kubevela/apis/types"
	"k8s.io/apimachinery/pkg/runtime"
	"reflect"
	"testing"
)

func TestAddSourceIntoDefinition(t *testing.T) {
	caseJson := []byte(`{"template":""}`)
	wantJson := []byte(`{"source":{"repoName":"foo"},"template":""}`)
	source := types.Source{RepoName: "foo"}
	testcase := runtime.RawExtension{Raw: caseJson}
	err := addSourceIntoExtension(&testcase, &source)
	if err != nil {
		t.Error("meet an error ", err)
		return
	}
	var result, want map[string]interface{}
	err = json.Unmarshal(testcase.Raw, &result)
	if err != nil {
		t.Error("marshaling object meet an error ", err)
		return
	}
	err = json.Unmarshal(wantJson, &want)
	if err != nil {
		t.Error("marshaling object meet an error ", err)
		return
	}
	if !reflect.DeepEqual(result, want) {
		t.Errorf("error result want %s, got %s", result, testcase)
	}
}

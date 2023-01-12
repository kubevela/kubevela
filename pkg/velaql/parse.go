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

package velaql

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/pkg/errors"

	"github.com/kubevela/workflow/pkg/cue/model/value"

	"github.com/oam-dev/kubevela/pkg/utils"
)

// QueryView contains query data
type QueryView struct {
	View      string
	Parameter map[string]interface{}
	Export    string
}

const (
	// PatternQL is the pattern string of velaQL, velaQL's query syntax is `ViewName{key1=value1 ,key2="value2",}.Export`
	PatternQL = `(?P<view>[a-z0-9](?:[a-z0-9\-]{0,61}[a-z0-9])?)(?P<parameter>{.*?})?\.?(?P<export>[_a-zA-Z][\._a-zA-Z0-9\[\]]*)?`
	// PatternKV is the pattern string of parameter
	PatternKV = `(?P<key>[^=]+)=(?P<value>[^=]*?)(?:,|$)`
	// KeyWordView represent view keyword
	KeyWordView = "view"
	// KeyWordParameter represent parameter keyword
	KeyWordParameter = "parameter"
	// KeyWordTemplate represents template keyword
	KeyWordTemplate = "template"
	// KeyWordExport represent export keyword
	KeyWordExport = "export"
	// DefaultExportValue is the default Export value
	DefaultExportValue = "status"
)

var (
	qlRegexp *regexp.Regexp
	kvRegexp *regexp.Regexp
)

func init() {
	qlRegexp = regexp.MustCompile(PatternQL)
	kvRegexp = regexp.MustCompile(PatternKV)
}

// ParseVelaQL parse velaQL to QueryView
func ParseVelaQL(ql string) (QueryView, error) {
	qv := QueryView{
		Export: DefaultExportValue,
	}

	groupNames := qlRegexp.SubexpNames()
	matched := qlRegexp.FindStringSubmatch(ql)
	if len(matched) != len(groupNames) || (len(matched) != 0 && matched[0] != ql) {
		return qv, errors.New("fail to parse the velaQL")
	}

	result := make(map[string]string, len(groupNames))
	for i, name := range groupNames {
		if i != 0 && name != "" {
			result[name] = strings.TrimSpace(matched[i])
		}
	}

	if len(result["view"]) == 0 {
		return qv, errors.New("view name shouldn't be empty")
	}

	qv.View = result[KeyWordView]
	if len(result[KeyWordExport]) != 0 {
		qv.Export = result[KeyWordExport]
	}
	var err error
	qv.Parameter, err = ParseParameter(result[KeyWordParameter])
	if err != nil {
		return qv, err
	}
	return qv, nil
}

// ParseVelaQLFromPath will parse a velaQL file path to QueryView
func ParseVelaQLFromPath(velaQLViewPath string) (*QueryView, error) {
	body, err := utils.ReadRemoteOrLocalPath(velaQLViewPath, false)
	if err != nil {
		return nil, errors.Errorf("read view file from %s: %v", velaQLViewPath, err)
	}

	val, err := value.NewValue(string(body), nil, "")
	if err != nil {
		return nil, errors.Errorf("new value for view: %v", err)
	}

	var expStr string
	exp, err := val.LookupValue(KeyWordExport)
	if err == nil {
		expStr, err = exp.String()
		if err != nil {
			expStr = DefaultExportValue
		}
	} else {
		expStr = DefaultExportValue
	}

	return &QueryView{
		View:      string(body),
		Parameter: nil,
		Export:    strings.Trim(strings.TrimSpace(expStr), `"`),
	}, nil
}

// ParseParameter parse parameter to map[string]interface{}
func ParseParameter(parameter string) (map[string]interface{}, error) {
	parameter = strings.TrimLeft(parameter, "{")
	parameter = strings.TrimRight(parameter, "}")
	parameter = strings.TrimSpace(parameter)

	if len(parameter) == 0 {
		return nil, errors.New("parameter shouldn't be empty")
	}

	groupNames := kvRegexp.SubexpNames()
	matchKVs := kvRegexp.FindAllStringSubmatch(parameter, -1)

	result := make(map[string]interface{}, len(matchKVs))
	for _, kv := range matchKVs {
		kvMap := make(map[string]string, 2)
		if len(kv) != len(groupNames) {
			return nil, errors.New("failed to parse the parameter")
		}

		for i, name := range groupNames {
			if i != 0 && name != "" {
				kvMap[name] = strings.TrimSpace(kv[i])
			}
		}

		if len(kvMap["key"]) == 0 || len(kvMap["value"]) == 0 {
			return nil, errors.New("key or value in parameter shouldn't be empty")
		}
		result[kvMap["key"]] = string2OtherType(kvMap["value"])
	}

	return result, nil
}

// string2OtherType convert string to other type
func string2OtherType(s string) interface{} {
	i, err := strconv.ParseInt(s, 10, 64)
	if err == nil {
		return i
	}

	b, err := strconv.ParseBool(s)
	if err == nil {
		return b
	}

	f, err := strconv.ParseFloat(s, 64)
	if err == nil {
		return f
	}
	return strings.Trim(s, "\"")
}

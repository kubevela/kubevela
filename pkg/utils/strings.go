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

package utils

import (
	"net/url"
	"path"
	"reflect"
	"sort"
	"strconv"
	"strings"
)

// StringsContain strings contain
func StringsContain(items []string, source string) bool {
	for _, item := range items {
		if item == source {
			return true
		}
	}
	return false
}

// ThreeWaySliceCompare will compare two string slice, with the three return values: [both in A and B], [only in A], [only in B]
func ThreeWaySliceCompare(a []string, b []string) ([]string, []string, []string) {
	m := make(map[string]struct{})
	for _, k := range b {
		m[k] = struct{}{}
	}

	var AB, AO, BO []string
	for _, k := range a {
		_, ok := m[k]
		if !ok {
			AO = append(AO, k)
			continue
		}
		AB = append(AB, k)
		delete(m, k)
	}
	for k := range m {
		BO = append(BO, k)
	}
	sort.Strings(AB)
	sort.Strings(AO)
	sort.Strings(BO)
	return AB, AO, BO
}

// EqualSlice checks if two slice are equal
func EqualSlice(a, b []string) bool {
	sort.Strings(a)
	sort.Strings(b)
	return reflect.DeepEqual(a, b)
}

// SliceIncludeSlice the a slice include the b slice
func SliceIncludeSlice(a, b []string) bool {
	if EqualSlice(a, b) {
		return true
	}
	for _, item := range b {
		if !StringsContain(a, item) {
			return false
		}
	}
	return true
}

// ToString convery an interface to a string type.
func ToString(i interface{}) string {
	if i == nil {
		return ""
	}
	v := reflect.ValueOf(i)
	if v.Kind() == reflect.Ptr && !v.IsNil() {
		v = v.Elem()
	}
	i = v.Interface()
	switch s := i.(type) {
	case string:
		return s
	case bool:
		return strconv.FormatBool(s)
	case float64:
		return strconv.FormatFloat(s, 'f', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(s), 'f', -1, 32)
	case int:
		return strconv.Itoa(s)
	case int64:
		return strconv.FormatInt(s, 10)
	case int32:
		return strconv.Itoa(int(s))
	case int16:
		return strconv.FormatInt(int64(s), 10)
	case int8:
		return strconv.FormatInt(int64(s), 10)
	case uint:
		return strconv.FormatUint(uint64(s), 10)
	case uint64:
		// nolint
		return strconv.FormatUint(uint64(s), 10)
	case uint32:
		return strconv.FormatUint(uint64(s), 10)
	case uint16:
		return strconv.FormatUint(uint64(s), 10)
	case uint8:
		return strconv.FormatUint(uint64(s), 10)
	case []byte:
		return string(s)
	case nil:
		return ""
	default:
		return ""
	}
}

// MapKey2Array convery map keys to array
func MapKey2Array(source map[string]string) []string {
	var list []string
	for k := range source {
		list = append(list, k)
	}
	return list
}

// GetBoxDrawingString get line drawing string, see https://en.wikipedia.org/wiki/Box-drawing_character
// nolint:gocyclo
func GetBoxDrawingString(up bool, down bool, left bool, right bool, padLeft int, padRight int) string {
	var c rune
	switch {
	case up && down && left && right:
		c = '┼'
	case up && down && left && !right:
		c = '┤'
	case up && down && !left && right:
		c = '├'
	case up && down && !left && !right:
		c = '│'
	case up && !down && left && right:
		c = '┴'
	case up && !down && left && !right:
		c = '┘'
	case up && !down && !left && right:
		c = '└'
	case up && !down && !left && !right:
		c = '╵'
	case !up && down && left && right:
		c = '┬'
	case !up && down && left && !right:
		c = '┐'
	case !up && down && !left && right:
		c = '┌'
	case !up && down && !left && !right:
		c = '╷'
	case !up && !down && left && right:
		c = '─'
	case !up && !down && left && !right:
		c = '╴'
	case !up && !down && !left && right:
		c = '╶'
	case !up && !down && !left && !right:
		c = ' '
	}
	sb := strings.Builder{}
	writePadding := func(connect bool, width int) {
		for i := 0; i < width; i++ {
			if connect {
				sb.WriteRune('─')
			} else {
				sb.WriteRune(' ')
			}
		}
	}
	writePadding(left, padLeft)
	sb.WriteRune(c)
	writePadding(right, padRight)
	return sb.String()
}

// JoinURL join baseURL and subPath to be new URL
func JoinURL(baseURL, subPath string) (string, error) {
	parsedURL, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}
	parsedURL.RawPath = path.Join(parsedURL.RawPath, subPath)
	parsedURL.Path = path.Join(parsedURL.Path, subPath)
	URL := parsedURL.String()
	return URL, nil
}

/*
Copyright 2026 The KubeVela Authors.

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

package propexpr

import "fmt"

// A properties blob is JSON-shaped - objects, arrays and scalars - and every
// pass over one is looking for the same thing: the string leaves, which are
// where expressions live. Walk reads them, Map rewrites them, and both address a
// leaf by the same dotted-and-indexed path, so a path reported by validation
// names the same place as one recorded during a render.

// JoinPath extends a property path with a map key.
func JoinPath(prefix, key string) string {
	if prefix == "" {
		return key
	}
	return prefix + "." + key
}

// IndexPath extends a property path with a list index.
func IndexPath(prefix string, i int) string {
	return fmt.Sprintf("%s[%d]", prefix, i)
}

// Walk visits every string leaf, carrying the path it was found at. It stops at
// the first error.
//
// Non-string leaves are skipped rather than reported: an expression is always a
// string, and a caller wanting every scalar wants a different function.
func Walk(node interface{}, path string, fn func(path, raw string) error) error {
	switch val := node.(type) {
	case map[string]interface{}:
		for k, child := range val {
			if err := Walk(child, JoinPath(path, k), fn); err != nil {
				return err
			}
		}
	case []interface{}:
		for i, child := range val {
			if err := Walk(child, IndexPath(path, i), fn); err != nil {
				return err
			}
		}
	case string:
		return fn(path, val)
	}
	return nil
}

// Map rebuilds a properties blob, replacing each string leaf with whatever fn
// returns for it.
//
// A new tree is built rather than the input rewritten. Callers hold on to what
// they pass - the binding properties on a render context are read by every
// component and trait of an Application - so consuming the input would make the
// second read of it see something different from the first.
func Map(node interface{}, path string, fn func(path, raw string) (interface{}, error)) (interface{}, error) {
	switch val := node.(type) {
	case map[string]interface{}:
		out := make(map[string]interface{}, len(val))
		for k, child := range val {
			mapped, err := Map(child, JoinPath(path, k), fn)
			if err != nil {
				return nil, err
			}
			out[k] = mapped
		}
		return out, nil
	case []interface{}:
		out := make([]interface{}, len(val))
		for i, child := range val {
			mapped, err := Map(child, IndexPath(path, i), fn)
			if err != nil {
				return nil, err
			}
			out[i] = mapped
		}
		return out, nil
	case string:
		return fn(path, val)
	default:
		return node, nil
	}
}

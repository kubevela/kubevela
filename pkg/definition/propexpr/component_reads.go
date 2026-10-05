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

import (
	"fmt"
	"strings"
)

// QualifiedKey holds, inside a delivered component, what each placement call
// returns, keyed by PlacementCall. A chained call is nested the same way:
// cluster("data").namespace("orders") is the "namespace:orders" entry inside the
// "cluster:data" entry. So every placement function is the same lookup, and
// which placement it names was settled before evaluation.
const QualifiedKey = "$q"

// The placement calls a component read may make.
const (
	// PlaceCluster names a cluster: component.db.cluster("data").
	PlaceCluster = "cluster"
	// PlaceNamespace names a namespace, in the reader's cluster unless a cluster
	// call precedes it: component.db.namespace("orders").
	PlaceNamespace = "namespace"
)

// PlacementCall names one placement call, fn("arg"), as it is recorded,
// delivered and looked up.
func PlacementCall(fn, arg string) string { return fn + ":" + arg }

// SplitPlacementCall is the inverse of PlacementCall.
func SplitPlacementCall(call string) (string, string) {
	fn, arg, _ := strings.Cut(call, ":")
	return fn, arg
}

// qualifierSegment marks a qualifier inside a read path, so the path stays one
// list of segments and every comparison over paths keeps working. A NUL cannot
// appear in a field an author could write.
const qualifierSegment = "\x00"

// QualifierSegment encodes a qualifier as a path segment.
func QualifierSegment(q string) string { return qualifierSegment + q }

// SegmentQualifier reports the qualifier a path segment encodes, if it is one.
func SegmentQualifier(segment string) (string, bool) {
	if strings.HasPrefix(segment, qualifierSegment) {
		return strings.TrimPrefix(segment, qualifierSegment), true
	}
	return "", false
}

// qualifierExpr renders a placement call the way an author writes it.
func qualifierExpr(call string) string {
	fn, arg := SplitPlacementCall(call)
	return fmt.Sprintf(".%s(%q)", fn, arg)
}

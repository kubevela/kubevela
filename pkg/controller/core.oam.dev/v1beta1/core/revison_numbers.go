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

package core

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
)

// DefinitionRevisionNumbersName is the ConfigMap, in each namespace that holds definitions,
// recording for each definition name there the highest revision number issued and the hashes of
// the newest ones. A definition's revisions go with it when it is deleted. With the record, one
// created again under the same name takes back the number of content it once had, so pins on it
// keep working, and never reuses a number for other content, so no pin binds to that.
const DefinitionRevisionNumbersName = "definition-revision-numbers"

// recordedHashes is how many of a definition's newest numbers keep their hash in the record.
const recordedHashes = 20

// revisionNumberKey is a definition's key in its namespace's record.
func revisionNumberKey(kind common.DefinitionType, name string) string {
	return string(kind) + "." + name
}

// revisionNumbers is a definition's entry in the record.
type revisionNumbers struct {
	// Latest is the highest number issued.
	Latest int64 `json:"latest"`
	// Hashes holds the revision hash of each of the newest numbers, keyed by number.
	Hashes map[string]string `json:"hashes,omitempty"`
}

// definitionNameLabel is the label CreateDefinitionRevision puts the definition's name under.
func definitionNameLabel(kind common.DefinitionType) string {
	switch kind {
	case common.ComponentType:
		return oam.LabelComponentDefinitionName
	case common.TraitType:
		return oam.LabelTraitDefinitionName
	case common.PolicyType:
		return oam.LabelPolicyDefinitionName
	case common.WorkflowStepType:
		return oam.LabelWorkflowStepDefinitionName
	case common.SourceType:
		return oam.LabelSourceDefinitionName
	}
	return ""
}

// revisionHistory is what is known of a definition name's revision numbers: its entry in the
// record, and its revisions still in the cluster (which an orphaning delete leaves behind).
type revisionHistory struct {
	recorded  revisionNumbers
	revisions []v1beta1.DefinitionRevision
}

// readRevisionNumbers reads a definition's entry in the record, live; a missing one is empty.
func readRevisionNumbers(ctx context.Context, cli client.Client, namespace string, kind common.DefinitionType, name string) (revisionNumbers, error) {
	var recorded revisionNumbers
	numbers := &corev1.ConfigMap{}
	if err := liveReader(cli).Get(ctx, client.ObjectKey{Namespace: namespace, Name: DefinitionRevisionNumbersName}, numbers); err != nil {
		return recorded, client.IgnoreNotFound(err)
	}
	if entry, ok := numbers.Data[revisionNumberKey(kind, name)]; ok {
		if err := json.Unmarshal([]byte(entry), &recorded); err != nil {
			return recorded, fmt.Errorf("revision numbers of %s %s/%s in ConfigMap %s/%s: %w", kind, namespace, name, numbers.Namespace, numbers.Name, err)
		}
	}
	return recorded, nil
}

// readRevisionHistory reads the record and the revisions live: a reconcile must see the revision
// another just made, which the cache may not hold yet.
func readRevisionHistory(ctx context.Context, cli client.Client, namespace string, kind common.DefinitionType, name string) (revisionHistory, error) {
	recorded, err := readRevisionNumbers(ctx, cli, namespace, kind, name)
	if err != nil {
		return revisionHistory{}, err
	}
	history := revisionHistory{recorded: recorded}
	if label := definitionNameLabel(kind); label != "" {
		revs := &v1beta1.DefinitionRevisionList{}
		if err := liveReader(cli).List(ctx, revs, client.InNamespace(namespace), client.MatchingLabels{label: name}); err != nil {
			return revisionHistory{}, err
		}
		history.revisions = revs.Items
	}
	return history, nil
}

// next is one past the highest number the name has had: in the definition's status, in the
// record, and among its revisions.
func (h revisionHistory) next(last *common.Revision) int64 {
	highest := h.recorded.Latest
	if last != nil {
		highest = max(highest, last.Revision)
	}
	for _, rev := range h.revisions {
		highest = max(highest, rev.Spec.Revision)
	}
	return highest + 1
}

// newest is the highest-numbered revision in the cluster, if any.
func (h revisionHistory) newest() *v1beta1.DefinitionRevision {
	var newest *v1beta1.DefinitionRevision
	for i := range h.revisions {
		if newest == nil || h.revisions[i].Spec.Revision > newest.Spec.Revision {
			newest = &h.revisions[i]
		}
	}
	return newest
}

// holding is the highest-numbered revision in the cluster with defRev's content, if any.
func (h revisionHistory) holding(defRev *v1beta1.DefinitionRevision) *v1beta1.DefinitionRevision {
	var found *v1beta1.DefinitionRevision
	for i := range h.revisions {
		if DeepEqualDefRevision(&h.revisions[i], defRev) && (found == nil || h.revisions[i].Spec.Revision > found.Spec.Revision) {
			found = &h.revisions[i]
		}
	}
	return found
}

// hashedNumber is the highest recorded number whose revision is gone and whose hash is hash, or 0.
// A hash is a release's own: one that adds a field to the definition types changes every hash, so
// content recorded by another release matches nothing and takes a new number.
func (h revisionHistory) hashedNumber(hash string) int64 {
	var found int64
	for key, recorded := range h.recorded.Hashes {
		n, err := strconv.ParseInt(key, 10, 64)
		if err != nil || recorded != hash || n <= found {
			continue
		}
		gone := true
		for _, rev := range h.revisions {
			gone = gone && rev.Spec.Revision != n
		}
		if gone {
			found = n
		}
	}
	return found
}

// recordRevisionNumber records revision n of a definition with its hash: the highest number
// never goes down, and only the newest recordedHashes keep their hash. It writes only on a change.
func recordRevisionNumber(ctx context.Context, cli client.Client, namespace string, kind common.DefinitionType, name string, n int64, hash string) error {
	key := revisionNumberKey(kind, name)
	return retry.OnError(retry.DefaultRetry, func(err error) bool {
		return apierrors.IsConflict(err) || apierrors.IsAlreadyExists(err)
	}, func() error {
		numbers := &corev1.ConfigMap{}
		err := liveReader(cli).Get(ctx, client.ObjectKey{Namespace: namespace, Name: DefinitionRevisionNumbersName}, numbers)
		if err != nil && !apierrors.IsNotFound(err) {
			return err
		}
		exists := err == nil
		var recorded revisionNumbers
		if entry, ok := numbers.Data[key]; ok {
			// an entry that does not parse is replaced
			_ = json.Unmarshal([]byte(entry), &recorded)
		}
		updated := withRevision(recorded, n, hash)
		if exists && reflect.DeepEqual(updated, recorded) {
			return nil
		}
		entry, err := json.Marshal(updated)
		if err != nil {
			return err
		}
		if !exists {
			return cli.Create(ctx, &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: DefinitionRevisionNumbersName},
				Data:       map[string]string{key: string(entry)},
			})
		}
		if numbers.Data == nil {
			numbers.Data = map[string]string{}
		}
		numbers.Data[key] = string(entry)
		return cli.Update(ctx, numbers)
	})
}

// withRevision is recorded with revision n and its hash added, trimmed to the newest hashes.
func withRevision(recorded revisionNumbers, n int64, hash string) revisionNumbers {
	updated := revisionNumbers{Latest: max(recorded.Latest, n), Hashes: map[string]string{}}
	for k, v := range recorded.Hashes {
		updated.Hashes[k] = v
	}
	if hash != "" {
		updated.Hashes[strconv.FormatInt(n, 10)] = hash
	}
	numbersOf := make([]int64, 0, len(updated.Hashes))
	for k := range updated.Hashes {
		if v, err := strconv.ParseInt(k, 10, 64); err == nil {
			numbersOf = append(numbersOf, v)
		} else {
			delete(updated.Hashes, k)
		}
	}
	sort.Slice(numbersOf, func(i, j int) bool { return numbersOf[i] > numbersOf[j] })
	for _, old := range numbersOf[min(len(numbersOf), recordedHashes):] {
		delete(updated.Hashes, strconv.FormatInt(old, 10))
	}
	if len(updated.Hashes) == 0 {
		updated.Hashes = nil
	}
	return updated
}

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

package apply

import (
	"encoding/json"
	"time"

	corev1 "k8s.io/api/core/v1"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/jsonmergepatch"
	"k8s.io/apimachinery/pkg/util/mergepatch"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/pkg/oam"
)

var k8sScheme = runtime.NewScheme()
var metadataAccessor = meta.NewAccessor()

func init() {
	_ = clientgoscheme.AddToScheme(k8sScheme)
}

// threeWayMergePatch creates a patch by computing a three way diff based on
// its current state, modified state, and last-applied-state recorded in the
// annotation.
func threeWayMergePatch(currentObj, modifiedObj client.Object, a *applyAction) (client.Patch, error) {
	current, err := json.Marshal(currentObj)
	if err != nil {
		return nil, err
	}
	original, err := getOriginalConfiguration(currentObj)
	if err != nil {
		return nil, err
	}
	modified, err := getModifiedConfiguration(modifiedObj, a.updateAnnotation)
	if err != nil {
		return nil, err
	}

	var patchType types.PatchType
	var patchData []byte
	var lookupPatchMeta strategicpatch.LookupPatchMeta

	versionedObject, err := k8sScheme.New(currentObj.GetObjectKind().GroupVersionKind())
	switch {
	case runtime.IsNotRegisteredError(err):
		// use JSONMergePatch for custom resources
		// because StrategicMergePatch doesn't support custom resources
		patchType = types.MergePatchType
		preconditions := []mergepatch.PreconditionFunc{
			mergepatch.RequireKeyUnchanged("apiVersion"),
			mergepatch.RequireKeyUnchanged("kind"),
			mergepatch.RequireMetadataKeyUnchanged("name")}
		patchData, err = jsonmergepatch.CreateThreeWayJSONMergePatch(original, modified, current, preconditions...)
		if err != nil {
			return nil, err
		}
	case err != nil:
		return nil, err
	default:
		// use StrategicMergePatch for K8s built-in resources
		patchType = types.StrategicMergePatchType
		lookupPatchMeta, err = strategicpatch.NewPatchMetaFromStruct(versionedObject)
		if err != nil {
			return nil, err
		}
		patchData, err = strategicpatch.CreateThreeWayMergePatch(original, modified, current, lookupPatchMeta, true)
		if err != nil {
			return nil, err
		}
	}
	return client.RawPatch(patchType, patchData), nil
}

// addLastAppliedConfigAnnotation creates annotation recording current configuration as
// original configuration for latter use in computing a three way diff
func addLastAppliedConfigAnnotation(obj runtime.Object) error {
	config, err := getModifiedConfiguration(obj, false)
	if err != nil {
		return err
	}
	annots, _ := metadataAccessor.Annotations(obj)
	if annots == nil {
		annots = make(map[string]string)
	}
	annots[oam.AnnotationLastAppliedConfig] = string(config)
	return metadataAccessor.SetAnnotations(obj, annots)
}

// getModifiedConfiguration serializes the object into byte stream.
// If `updateAnnotation` is true, it embeds the result as an annotation in the
// modified configuration.
func getModifiedConfiguration(obj runtime.Object, updateAnnotation bool) ([]byte, error) {
	annots, err := metadataAccessor.Annotations(obj)
	if err != nil {
		return nil, errors.Wrap(err, "cannot access metadata.annotations")
	}
	if annots == nil {
		annots = make(map[string]string)
	}

	original := annots[oam.AnnotationLastAppliedConfig]
	// remove the annotation to avoid recursion
	delete(annots, oam.AnnotationLastAppliedConfig)
	_ = metadataAccessor.SetAnnotations(obj, annots)
	// do not include an empty map
	if len(annots) == 0 {
		_ = metadataAccessor.SetAnnotations(obj, nil)
	}

	var modified []byte
	modified, err = json.Marshal(obj)
	if err != nil {
		return nil, err
	}

	if updateAnnotation {
		annots[oam.AnnotationLastAppliedConfig] = string(modified)
		err = metadataAccessor.SetAnnotations(obj, annots)
		if err != nil {
			return nil, err
		}
		modified, err = json.Marshal(obj)
		if err != nil {
			return nil, err
		}
	}

	// restore original annotations back to the object
	annots[oam.AnnotationLastAppliedConfig] = original
	annots[oam.AnnotationLastAppliedTime] = time.Now().Format(time.RFC3339)
	_ = metadataAccessor.SetAnnotations(obj, annots)
	return modified, nil
}

// getOriginalConfiguration gets original configuration of the object
// form the annotation, or nil if no annotation found.
func getOriginalConfiguration(obj runtime.Object) ([]byte, error) {
	annots, err := metadataAccessor.Annotations(obj)
	if err != nil {
		return nil, errors.Wrap(err, "cannot access metadata.annotations")
	}
	if annots == nil {
		return nil, nil
	}

	oamOriginal, oamOk := annots[oam.AnnotationLastAppliedConfig]
	if oamOk {
		if oamOriginal == "-" || oamOriginal == "skip" {
			return nil, nil
		}
		return []byte(oamOriginal), nil
	}

	kubectlOriginal, kubectlOK := annots[corev1.LastAppliedConfigAnnotation]
	if kubectlOK {
		return []byte(kubectlOriginal), nil
	}
	return nil, nil
}

func isEmptyPatch(patch client.Patch) bool {
	if patch == nil {
		return true
	}
	data, _ := patch.Data(nil)
	return data != nil && string(data) == "{}"
}

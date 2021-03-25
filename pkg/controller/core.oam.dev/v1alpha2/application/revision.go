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

package application

import (
	"context"

	"github.com/pkg/errors"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/controller/utils"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

// AppRevisionHash is used to compute the hash value of the AppRevision
type AppRevisionHash struct {
	ApplicationSpecHash     string
	WorkloadDefinitionHash  map[string]string
	ComponentDefinitionHash map[string]string
	TraitDefinitionHash     map[string]string
	ScopeDefinitionHash     map[string]string
}

// GenerateRevision will generate revision for an Application when created/updated
func (h *appHandler) GenerateRevision(ctx context.Context, ac *v1alpha2.ApplicationConfiguration,
	comps []*v1alpha2.Component) (bool, *v1beta1.ApplicationRevision, error) {
	copiedApp := h.app.DeepCopy()
	// We better to remove all object status in the appRevision
	copiedApp.Status = common.AppStatus{}
	appRev := &v1beta1.ApplicationRevision{
		Spec: v1beta1.ApplicationRevisionSpec{
			Application:              *copiedApp,
			Components:               ConvertComponent2RawRevision(comps),
			ApplicationConfiguration: util.Object2RawExtension(ac),
			ComponentDefinitions:     make(map[string]v1beta1.ComponentDefinition),
			WorkloadDefinitions:      make(map[string]v1beta1.WorkloadDefinition),
			TraitDefinitions:         make(map[string]v1beta1.TraitDefinition),
			ScopeDefinitions:         make(map[string]v1beta1.ScopeDefinition),
		},
	}
	// appRev should have the same annotation/label as the app
	appRev.Namespace = h.app.Namespace
	appRev.SetAnnotations(h.app.GetAnnotations())
	appRev.SetLabels(h.app.GetLabels())
	appRev.SetOwnerReferences([]metav1.OwnerReference{{
		APIVersion: v1alpha2.SchemeGroupVersion.String(),
		Kind:       v1alpha2.ApplicationKind,
		Name:       h.app.Name,
		UID:        h.app.UID,
		Controller: pointer.BoolPtr(false),
	}})
	for _, w := range h.appfile.Workloads {
		if w == nil {
			continue
		}
		if w.FullTemplate.ComponentDefinition != nil {
			cd := w.FullTemplate.ComponentDefinition.DeepCopy()
			cd.Status = v1beta1.ComponentDefinitionStatus{}
			appRev.Spec.ComponentDefinitions[w.FullTemplate.ComponentDefinition.Name] = *cd
		}
		if w.FullTemplate.WorkloadDefinition != nil {
			wd := w.FullTemplate.WorkloadDefinition.DeepCopy()
			wd.Status = v1beta1.WorkloadDefinitionStatus{}
			appRev.Spec.WorkloadDefinitions[w.FullTemplate.WorkloadDefinition.Name] = *wd
		}
		for _, t := range w.Traits {
			if t == nil {
				continue
			}
			if t.FullTemplate.TraitDefinition != nil {
				td := t.FullTemplate.TraitDefinition.DeepCopy()
				td.Status = v1beta1.TraitDefinitionStatus{}
				appRev.Spec.TraitDefinitions[t.FullTemplate.TraitDefinition.Name] = *td
			}
		}
	}
	appRevisionHash, err := ComputeAppRevisionHash(appRev)
	if err != nil {
		return false, nil, err
	}
	util.AddLabels(appRev, map[string]string{oam.LabelAppRevisionHash: appRevisionHash})

	// check if the appRevision is different from the existing one
	if h.app.Status.LatestRevision != nil && h.app.Status.LatestRevision.RevisionHash == appRevisionHash {
		// get the last revision and double check
		lastAppRevision := &v1beta1.ApplicationRevision{}
		if err := h.r.Get(ctx, client.ObjectKey{Name: h.app.Status.LatestRevision.Name,
			Namespace: h.app.Namespace}, lastAppRevision); err != nil {
			return false, nil, errors.Wrapf(err, "fail to get applicationRevision %s", h.app.Status.LatestRevision.Name)
		}
		if DeepEqualRevision(lastAppRevision, appRev) {
			// No difference on spec, will not create a new revision
			appRev.Name = lastAppRevision.Name
			appRev.ResourceVersion = lastAppRevision.ResourceVersion
			return false, appRev, nil
		}
	}
	// need to create a new appRev
	var revision int64
	appRev.Name, revision = utils.GetAppNextRevision(h.app)
	h.app.Status.LatestRevision = &common.Revision{
		Name:         appRev.Name,
		Revision:     revision,
		RevisionHash: appRevisionHash,
	}
	// make sure that we persist the latest revision first
	if err = h.r.UpdateStatus(ctx, h.app); err != nil {
		return false, nil, err
	}
	h.logger.Info("recorded the latest appConfig revision", "application name", h.app.GetName(),
		"latest revision", appRev.Name)
	return true, appRev, nil
}

// ConvertComponent2RawRevision convert to ComponentMap
func ConvertComponent2RawRevision(comps []*v1alpha2.Component) []common.RawComponent {
	var objs []common.RawComponent
	for _, comp := range comps {
		obj := comp.DeepCopy()
		objs = append(objs, common.RawComponent{
			Raw: util.Object2RawExtension(obj),
		})
	}
	return objs
}

// DeepEqualRevision will check the Application and Definition to see if the Application is the same revision
// AC and component are generated by the application and definitions
func DeepEqualRevision(old, new *v1beta1.ApplicationRevision) bool {
	if len(old.Spec.WorkloadDefinitions) != len(new.Spec.WorkloadDefinitions) {
		return false
	}
	if len(old.Spec.TraitDefinitions) != len(new.Spec.TraitDefinitions) {
		return false
	}
	if len(old.Spec.ComponentDefinitions) != len(new.Spec.ComponentDefinitions) {
		return false
	}
	if len(old.Spec.ScopeDefinitions) != len(new.Spec.ScopeDefinitions) {
		return false
	}
	for key, wd := range new.Spec.WorkloadDefinitions {
		if !apiequality.Semantic.DeepEqual(old.Spec.WorkloadDefinitions[key].Spec, wd.Spec) {
			return false
		}
	}
	for key, cd := range new.Spec.ComponentDefinitions {
		if !apiequality.Semantic.DeepEqual(old.Spec.ComponentDefinitions[key].Spec, cd.Spec) {
			return false
		}
	}
	for key, td := range new.Spec.TraitDefinitions {
		if !apiequality.Semantic.DeepEqual(old.Spec.TraitDefinitions[key].Spec, td.Spec) {
			return false
		}
	}
	for key, sd := range new.Spec.ScopeDefinitions {
		if !apiequality.Semantic.DeepEqual(old.Spec.ScopeDefinitions[key].Spec, sd.Spec) {
			return false
		}
	}
	return apiequality.Semantic.DeepEqual(&old.Spec.Application.Spec, &new.Spec.Application.Spec)
}

// ComputeAppRevisionHash computes a single hash value for an appRevision object
func ComputeAppRevisionHash(appRevision *v1beta1.ApplicationRevision) (string, error) {
	// we first constructs a AppRevisionHash structure to store all the meaningful spec hashes
	// and avoid computing the annotations. Those fields are all read from k8s already so their
	// raw extension value are already byte array. Never include any in-memory objects.
	appRevisionHash := AppRevisionHash{
		WorkloadDefinitionHash:  make(map[string]string),
		ComponentDefinitionHash: make(map[string]string),
		TraitDefinitionHash:     make(map[string]string),
		ScopeDefinitionHash:     make(map[string]string),
	}
	var err error
	appRevisionHash.ApplicationSpecHash, err = utils.ComputeSpecHash(&appRevision.Spec.Application.Spec)
	if err != nil {
		return "", err
	}
	for key, wd := range appRevision.Spec.WorkloadDefinitions {
		hash, err := utils.ComputeSpecHash(&wd.Spec)
		if err != nil {
			return "", err
		}
		appRevisionHash.WorkloadDefinitionHash[key] = hash
	}
	for key, cd := range appRevision.Spec.ComponentDefinitions {
		hash, err := utils.ComputeSpecHash(&cd.Spec)
		if err != nil {
			return "", err
		}
		appRevisionHash.ComponentDefinitionHash[key] = hash
	}
	for key, td := range appRevision.Spec.TraitDefinitions {
		hash, err := utils.ComputeSpecHash(&td.Spec)
		if err != nil {
			return "", err
		}
		appRevisionHash.TraitDefinitionHash[key] = hash
	}
	for key, sd := range appRevision.Spec.ScopeDefinitions {
		hash, err := utils.ComputeSpecHash(&sd.Spec)
		if err != nil {
			return "", err
		}
		appRevisionHash.TraitDefinitionHash[key] = hash
	}
	// compute the hash of the entire structure
	return utils.ComputeSpecHash(&appRevisionHash)
}

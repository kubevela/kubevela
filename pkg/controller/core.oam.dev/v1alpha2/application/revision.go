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
	"sort"

	"github.com/pkg/errors"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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

// UpdateRevisionStatus will update the status of Application object mainly for update the revision part
func (h *appHandler) UpdateRevisionStatus(ctx context.Context, revName, hash string, revision int64) error {
	h.app.Status.LatestRevision = &common.Revision{
		Name:         revName,
		Revision:     revision,
		RevisionHash: hash,
	}
	// make sure that we persist the latest revision first
	if err := h.r.UpdateStatus(ctx, h.app); err != nil {
		h.logger.Error(err, "update the latest appConfig revision to status", "application name", h.app.GetName(),
			"latest revision", revName)
		return err
	}
	h.logger.Info("recorded the latest appConfig revision", "application name", h.app.GetName(),
		"latest revision", revName)
	return nil
}

// setRevisionMetadata will set the ApplicationRevision with the same annotation/label as the app
func (h *appHandler) setRevisionMetadata(appRev *v1beta1.ApplicationRevision) {
	appRev.Namespace = h.app.Namespace
	appRev.SetAnnotations(h.app.GetAnnotations())
	appRev.SetLabels(h.app.GetLabels())
	util.AddLabels(appRev, map[string]string{oam.LabelAppRevisionHash: h.revisionHash})
	appRev.SetOwnerReferences([]metav1.OwnerReference{{
		APIVersion: v1beta1.SchemeGroupVersion.String(),
		Kind:       v1beta1.ApplicationKind,
		Name:       h.app.Name,
		UID:        h.app.UID,
		Controller: pointer.BoolPtr(false),
	}})
}

// setRevisionWithRenderedResult will set the ApplicationRevision with the rendered result
// it's ApplicationConfiguration and Component for now
func (h *appHandler) setRevisionWithRenderedResult(appRev *v1beta1.ApplicationRevision, ac *v1alpha2.ApplicationConfiguration,
	comps []*v1alpha2.Component) {
	appRev.Spec.Components = ConvertComponent2RawRevision(comps)
	appRev.Spec.ApplicationConfiguration = util.Object2RawExtension(ac)
}

// gatherRevisionSpec will gather all revision spec withouth metadata and rendered result.
// the gathered Revision spec will be enough to calculate the hash and compare with the old revision
func (h *appHandler) gatherRevisionSpec() (*v1beta1.ApplicationRevision, string, error) {
	copiedApp := h.app.DeepCopy()
	// We better to remove all object status in the appRevision
	copiedApp.Status = common.AppStatus{}
	appRev := &v1beta1.ApplicationRevision{
		Spec: v1beta1.ApplicationRevisionSpec{
			Application:          *copiedApp,
			ComponentDefinitions: make(map[string]v1beta1.ComponentDefinition),
			WorkloadDefinitions:  make(map[string]v1beta1.WorkloadDefinition),
			TraitDefinitions:     make(map[string]v1beta1.TraitDefinition),
			ScopeDefinitions:     make(map[string]v1beta1.ScopeDefinition),
		},
	}
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
		// TODO(wonderflow): take scope into the revision
	}
	appRevisionHash, err := ComputeAppRevisionHash(appRev)
	if err != nil {
		h.logger.Error(err, "compute hash of appRevision for application", "application name", h.app.GetName())
		return appRev, "", err
	}
	return appRev, appRevisionHash, nil
}

// compareWithLastRevisionSpec will get the last AppRevision from K8s and compare the Application and Definition's Spec
func (h *appHandler) compareWithLastRevisionSpec(ctx context.Context, newAppRevisionHash string, newAppRevision *v1beta1.ApplicationRevision) (bool, error) {

	// the last revision doesn't exist.
	if h.app.Status.LatestRevision == nil {
		return true, nil
	}
	// the hash value doesn't align
	if h.app.Status.LatestRevision.RevisionHash != newAppRevisionHash {
		return true, nil
	}

	// check if the appRevision is deep equal in Spec level
	// get the last revision from K8s and double check
	lastAppRevision := &v1beta1.ApplicationRevision{}
	if err := h.r.Get(ctx, client.ObjectKey{Name: h.app.Status.LatestRevision.Name,
		Namespace: h.app.Namespace}, lastAppRevision); err != nil {
		h.logger.Error(err, "get the last appRevision from K8s", "application name",
			h.app.GetName(), "revision", h.app.Status.LatestRevision.Name)
		return false, errors.Wrapf(err, "fail to get applicationRevision %s", h.app.Status.LatestRevision.Name)
	}
	if DeepEqualRevision(lastAppRevision, newAppRevision) {
		// No difference on spec, will not create a new revision
		// align the name and resourceVersion
		newAppRevision.Name = lastAppRevision.Name
		newAppRevision.ResourceVersion = lastAppRevision.ResourceVersion
		return false, nil
	}
	// if reach here, it's same hash but different spec
	return true, nil
}

// GenerateAppRevision will generate a pure revision without metadata and rendered result
// the generated revision will be compare with the last revision to see if there's any difference.
func (h *appHandler) GenerateAppRevision(ctx context.Context) (*v1beta1.ApplicationRevision, error) {
	appRev, appRevisionHash, err := h.gatherRevisionSpec()
	if err != nil {
		return nil, err
	}
	isNewRev, err := h.compareWithLastRevisionSpec(ctx, appRevisionHash, appRev)
	if err != nil {
		return appRev, err
	}
	if isNewRev {
		appRev.Name, _ = utils.GetAppNextRevision(h.app)
	}
	h.isNewRevision = isNewRev
	h.revisionHash = appRevisionHash
	return appRev, nil
}

// FinalizeAppRevision will finalize the AppRevision with metadata and rendered result revision for an Application when created/updated
func (h *appHandler) FinalizeAppRevision(appRev *v1beta1.ApplicationRevision,
	ac *v1alpha2.ApplicationConfiguration, comps []*v1alpha2.Component) {

	h.setRevisionMetadata(appRev)
	h.setRevisionWithRenderedResult(appRev, ac, comps)

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

// DeepEqualRevision will compare the spec of Application and Definition to see if the Application is the same revision
// Spec of AC and Component will not be compared as they are generated by the application and definitions
// Note the Spec compare can only work when the RawExtension are decoded well in the RawExtension.Object instead of in RawExtension.Raw(bytes)
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
// Spec of Application/WorkloadDefinitions/ComponentDefinitions/TraitDefinitions/ScopeDefinitions will be taken into compute
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
		appRevisionHash.ScopeDefinitionHash[key] = hash
	}
	// compute the hash of the entire structure
	return utils.ComputeSpecHash(&appRevisionHash)
}

// cleanUpApplicationRevision check all appRevisions of the application, remove them if the number of them exceed the limit
func cleanUpApplicationRevision(ctx context.Context, h *appHandler) error {
	listOpts := []client.ListOption{
		client.InNamespace(h.app.Namespace),
		client.MatchingLabels{oam.LabelAppName: h.app.Name},
	}
	appRevisionList := new(v1beta1.ApplicationRevisionList)
	// controller-runtime will cache all appRevision by default, there is no need to watch or own appRevision in manager
	if err := h.r.List(ctx, appRevisionList, listOpts...); err != nil {
		return err
	}
	usingRevision, err := gatherUsingAppRevision(ctx, h)
	if err != nil {
		return err
	}
	needKill := len(appRevisionList.Items) - h.r.appRevisionLimit - len(usingRevision)
	if needKill <= 0 {
		return nil
	}
	h.logger.Info("application controller cleanup old appRevisions", "needKillNum", needKill)
	sortedRevision := appRevisionList.Items
	sort.Sort(historiesByRevision(sortedRevision))

	for _, rev := range sortedRevision {
		if needKill <= 0 {
			break
		}
		// we shouldn't delete the revision witch appContext pointing to
		if usingRevision[rev.Name] {
			continue
		}
		if err := h.r.Delete(ctx, rev.DeepCopy()); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
		needKill--
	}
	return nil
}

// gatherUsingAppRevision get all using appRevisions include app's status pointing to and appContext point to
func gatherUsingAppRevision(ctx context.Context, h *appHandler) (map[string]bool, error) {
	listOpts := []client.ListOption{
		client.InNamespace(h.app.Namespace),
		client.MatchingLabels{oam.LabelAppName: h.app.Name},
	}
	usingRevision := map[string]bool{}
	if h.app.Status.LatestRevision != nil && len(h.app.Status.LatestRevision.Name) != 0 {
		usingRevision[h.app.Status.LatestRevision.Name] = true
	}
	appContextList := new(v1alpha2.ApplicationContextList)
	err := h.r.List(ctx, appContextList, listOpts...)
	if err != nil {
		return nil, err
	}
	for _, appContext := range appContextList.Items {
		usingRevision[appContext.Spec.ApplicationRevisionName] = true
	}
	appDeployUsingRevision, err := utils.CheckAppDeploymentUsingAppRevision(ctx, h.r, h.app.Namespace, h.app.Name)
	if err != nil {
		return usingRevision, err
	}
	for _, revName := range appDeployUsingRevision {
		usingRevision[revName] = true
	}
	return usingRevision, nil
}

type historiesByRevision []v1beta1.ApplicationRevision

func (h historiesByRevision) Len() int      { return len(h) }
func (h historiesByRevision) Swap(i, j int) { h[i], h[j] = h[j], h[i] }
func (h historiesByRevision) Less(i, j int) bool {
	// the appRevision is generated by vela,  the error always is nil, so ignore it
	ir, _ := util.ExtractRevisionNum(h[i].Name)
	ij, _ := util.ExtractRevisionNum(h[j].Name)
	return ir < ij
}

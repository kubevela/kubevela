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
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"strings"

	authv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/oam-dev/kubevela/pkg/oam/util"
	velaerr "github.com/oam-dev/kubevela/pkg/utils/errors"
)

// MutateOption defines the function pattern for mutate
type MutateOption func(object metav1.Object) error

// MergeOverrideLabels will merge the existing labels and override by the labels passed in
func MergeOverrideLabels(labels map[string]string) MutateOption {
	return func(object metav1.Object) error {
		util.AddLabels(object, labels)
		return nil
	}
}

// MergeOverrideAnnotations will merge the existing annotations and override by the annotations passed in
func MergeOverrideAnnotations(annotations map[string]string) MutateOption {
	return func(object metav1.Object) error {
		util.AddAnnotations(object, annotations)
		return nil
	}
}

// MergeNoConflictLabels will merge the existing labels with the labels passed in, it will report conflicts if exists
func MergeNoConflictLabels(labels map[string]string) MutateOption {
	return func(object metav1.Object) error {
		existingLabels := object.GetLabels()
		// check and fill the labels
		for k, v := range labels {
			ev, ok := existingLabels[k]
			if ok && ev != "" && ev != v {
				return fmt.Errorf("%s for object %s, key: %s, conflicts value: %s <-> %s", velaerr.LabelConflict, object.GetName(), k, ev, v)
			}
			existingLabels[k] = v
		}
		object.SetLabels(existingLabels)
		return nil
	}
}

// CreateOrUpdateNamespace will create a namespace if not exist, it will also update a namespace if exists
// It will report an error if the labels conflict while it will override the annotations
func CreateOrUpdateNamespace(ctx context.Context, kubeClient client.Client, name string, options ...MutateOption) error {
	err := CreateNamespace(ctx, kubeClient, name, options...)
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return err
	}
	return UpdateNamespace(ctx, kubeClient, name, options...)
}

// CreateNamespace will create a namespace with mutate option
func CreateNamespace(ctx context.Context, kubeClient client.Client, name string, options ...MutateOption) error {
	obj := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: corev1.NamespaceSpec{},
	}
	for _, op := range options {
		if err := op(obj); err != nil {
			return err
		}
	}
	return kubeClient.Create(ctx, obj)
}

// GetNamespace will return a namespace with mutate option
func GetNamespace(ctx context.Context, kubeClient client.Client, name string) (*corev1.Namespace, error) {
	obj := &corev1.Namespace{}
	err := kubeClient.Get(ctx, client.ObjectKey{Name: name}, obj)
	if err != nil {
		return nil, err
	}
	return obj, nil
}

// UpdateNamespace will update a namespace with mutate option
func UpdateNamespace(ctx context.Context, kubeClient client.Client, name string, options ...MutateOption) error {
	var namespace corev1.Namespace
	err := kubeClient.Get(ctx, k8stypes.NamespacedName{Name: name}, &namespace)
	if err != nil {
		return err
	}
	for _, op := range options {
		if err = op(&namespace); err != nil {
			return err
		}
	}
	return kubeClient.Update(ctx, &namespace)
}

// GetServiceAccountSubjectFromConfig extract ServiceAccountName subject from token
func GetServiceAccountSubjectFromConfig(cfg *rest.Config) string {
	sub, _ := GetTokenSubject(cfg.BearerToken)
	return sub
}

// GetCertificateCommonNameAndOrganizationsFromConfig extract CommonName and Organizations from Certificate
func GetCertificateCommonNameAndOrganizationsFromConfig(cfg *rest.Config) (string, []string) {
	cert := cfg.CertData
	if len(cert) == 0 && cfg.CertFile != "" {
		cert, _ = ioutil.ReadFile(cfg.CertFile)
	}
	name, _ := GetCertificateSubject(cert)
	if name == nil {
		return "", nil
	}
	return name.CommonName, name.Organization
}

// GetUserInfoFromConfig extract UserInfo from KubeConfig
func GetUserInfoFromConfig(cfg *rest.Config) *authv1.UserInfo {
	if sub := GetServiceAccountSubjectFromConfig(cfg); sub != "" {
		return &authv1.UserInfo{Username: sub}
	}
	if cn, orgs := GetCertificateCommonNameAndOrganizationsFromConfig(cfg); cn != "" {
		return &authv1.UserInfo{Username: cn, Groups: orgs}
	}
	return nil
}

// AutoSetSelfImpersonationInConfig set impersonate username and group to the identity in the original rest config
func AutoSetSelfImpersonationInConfig(cfg *rest.Config) {
	if userInfo := GetUserInfoFromConfig(cfg); userInfo != nil {
		cfg.Impersonate.UserName = userInfo.Username
		cfg.Impersonate.Groups = append(cfg.Impersonate.Groups, userInfo.Groups...)
	}
}

// CreateOrUpdate create or update a kubernetes object
func CreateOrUpdate(ctx context.Context, cli client.Client, obj client.Object) (controllerutil.OperationResult, error) {
	bs, err := json.Marshal(obj)
	if err != nil {
		return controllerutil.OperationResultNone, err
	}
	return controllerutil.CreateOrUpdate(ctx, cli, obj, func() error {
		createTimestamp := obj.GetCreationTimestamp()
		resourceVersion := obj.GetResourceVersion()
		deletionTimestamp := obj.GetDeletionTimestamp()
		generation := obj.GetGeneration()
		managedFields := obj.GetManagedFields()
		if e := json.Unmarshal(bs, obj); err != nil {
			return e
		}
		obj.SetCreationTimestamp(createTimestamp)
		obj.SetResourceVersion(resourceVersion)
		obj.SetDeletionTimestamp(deletionTimestamp)
		obj.SetGeneration(generation)
		obj.SetManagedFields(managedFields)
		return nil
	})
}

// EscapeResourceNameToLabelValue parse characters in resource name to label valid name
func EscapeResourceNameToLabelValue(resourceName string) string {
	return strings.ReplaceAll(resourceName, ":", "_")
}

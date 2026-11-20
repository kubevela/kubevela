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

package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
)

const sensitivePinsKey = "pins"

// secretPins keeps the sensitive fields of a workflow run's source pins in a Secret the
// Application owns, workflow-<app>-sources, beside the workflow-<app>-context ConfigMap that
// holds the rest.
type secretPins struct {
	client client.Client
	// reader reads past the cache, which may not yet hold the Secret a step before wrote; nil
	// reads through client.
	reader client.Reader
	app    *v1beta1.Application
}

type sealedPins struct {
	Run    string                 `json:"run"`
	Fields map[string]interface{} `json:"fields"`
}

func (s secretPins) key() client.ObjectKey {
	return client.ObjectKey{Namespace: s.app.Namespace, Name: pinsSecretName(s.app.Name)}
}

// pinsSecretName is workflow-<app>-sources, with a long Application name cut short and a hash of
// it kept, so the name stays within the 253 characters a Secret's may have and unique to the app.
func pinsSecretName(app string) string {
	const prefix, suffix, limit = "workflow-", "-sources", 253
	if name := prefix + app + suffix; len(name) <= limit {
		return name
	}
	sum := sha256.Sum256([]byte(app))
	hash := hex.EncodeToString(sum[:])[:8]
	kept := strings.TrimRight(app[:limit-len(prefix)-len(suffix)-len(hash)-1], ".-")
	return prefix + kept + "-" + hash + suffix
}

// get reads the pins Secret live when a reader is set.
func (s secretPins) get(ctx context.Context, secret *corev1.Secret) error {
	if s.reader != nil {
		return s.reader.Get(ctx, s.key(), secret)
	}
	return s.client.Get(ctx, s.key(), secret)
}

// controls reports whether the Application controls secret, which pins are read from and written
// to only then: a Secret of that name it does not control may be a restored copy or another's.
func (s secretPins) controls(secret *corev1.Secret) bool {
	owner := metav1.GetControllerOf(secret)
	return owner != nil && owner.UID == s.app.UID
}

// Ref points at a pinned field: the Secret, the data key the run's pins are stored under, and
// the field within them.
func (s secretPins) Ref(key string) string {
	return fmt.Sprintf("secret:%s/%s#%s", s.key().Name, sensitivePinsKey, key)
}

// Load returns the fields pinned for run, or nothing when the Secret holds another run's or none.
func (s secretPins) Load(ctx context.Context, run string) (map[string]interface{}, error) {
	secret := &corev1.Secret{}
	if err := s.get(ctx, secret); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	if !s.controls(secret) {
		return nil, nil
	}
	var pins sealedPins
	if err := json.Unmarshal(secret.Data[sensitivePinsKey], &pins); err != nil || pins.Run != run {
		return nil, nil //nolint:nilerr // an unreadable pin is resolved again
	}
	return pins.Fields, nil
}

// Save replaces the Secret's fields with run's.
func (s secretPins) Save(ctx context.Context, run string, fields map[string]interface{}) error {
	raw, err := json.Marshal(sealedPins{Run: run, Fields: fields})
	if err != nil {
		return err
	}
	secret := &corev1.Secret{}
	err = s.get(ctx, secret)
	if apierrors.IsNotFound(err) {
		secret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      s.key().Name,
				Namespace: s.key().Namespace,
				Labels:    map[string]string{oam.LabelAppName: s.app.Name, oam.LabelAppNamespace: s.app.Namespace},
				OwnerReferences: []metav1.OwnerReference{*metav1.NewControllerRef(s.app,
					v1beta1.SchemeGroupVersion.WithKind(v1beta1.ApplicationKind))},
			},
			Type: corev1.SecretTypeOpaque,
			Data: map[string][]byte{sensitivePinsKey: raw},
		}
		if err = s.client.Create(ctx, secret); !apierrors.IsAlreadyExists(err) {
			return err
		}
		// Created since it was read: update it as found.
		secret = &corev1.Secret{}
		err = s.get(ctx, secret)
	}
	if err != nil {
		return err
	}
	if !s.controls(secret) {
		return fmt.Errorf("secret %s/%s is not controlled by Application %s/%s", secret.Namespace, secret.Name, s.app.Namespace, s.app.Name)
	}
	secret.Data = map[string][]byte{sensitivePinsKey: raw}
	return s.client.Update(ctx, secret)
}

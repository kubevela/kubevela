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

package addon

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	velatypes "github.com/oam-dev/kubevela/apis/types"
)

const registryConfigMapName = "vela-addon-registry"
const registriesKey = "registries"
const tokenSecretNamePrefix = "addon-registry-"

// TokenSource is an interface for addon source that has token
type TokenSource interface {
	// GetToken return the token of the source
	GetToken() string
	// SetToken set the token of the source
	SetToken(string)
	// SetTokenSecretRef set the token secret ref to the source
	SetTokenSecretRef(string)
	// GetTokenSecretRef return the token secret ref of the source
	GetTokenSecretRef() string
}

// GetTokenSource return the token source of the registry
func (r *Registry) GetTokenSource() TokenSource {
	if r.Git != nil {
		return r.Git
	}
	if r.Gitee != nil {
		return r.Gitee
	}
	if r.Gitlab != nil {
		return r.Gitlab
	}
	// Only an oci:// Helm source is secret backed. An http(s):// Helm repository
	// keeps its password in the ConfigMap, which is the behaviour released
	// versions already have; returning it here would start rewriting those
	// records into Secrets as a side effect of this refactor.
	if r.Helm != nil && IsOCIURL(r.Helm.URL) {
		return r.Helm
	}
	return nil
}

// Registry represent a addon registry model
type Registry struct {
	Name string `json:"name"`

	Helm   *HelmSource        `json:"helm,omitempty"`
	Git    *GitAddonSource    `json:"git,omitempty"`
	OSS    *OSSAddonSource    `json:"oss,omitempty"`
	Gitee  *GiteeAddonSource  `json:"gitee,omitempty"`
	Gitlab *GitlabAddonSource `json:"gitlab,omitempty"`
}

// RegistryDataStore CRUD addon registry data in configmap
type RegistryDataStore interface {
	ListRegistries(context.Context) ([]Registry, error)
	AddRegistry(context.Context, Registry) error
	DeleteRegistry(context.Context, string) error
	UpdateRegistry(context.Context, Registry) error
	GetRegistry(context.Context, string) (Registry, error)
}

// NewRegistryDataStore get RegistryDataStore operation interface
func NewRegistryDataStore(cli client.Client) RegistryDataStore {
	return registryImpl{cli, registryConfigMapName, tokenSecretNamePrefix}
}

// NewRegistryDataStoreFor returns a RegistryDataStore backed by the ConfigMap named
// cmName, storing registry tokens in secrets named secretNamePrefix + registry name.
// A module registry store uses this so its entries and credentials stay separate from
// the addon registry store, while sharing one implementation.
func NewRegistryDataStoreFor(cli client.Client, cmName, secretNamePrefix string) RegistryDataStore {
	return registryImpl{cli, cmName, secretNamePrefix}
}

type registryImpl struct {
	client client.Client
	// cmName is the ConfigMap holding this store's registries.
	cmName string
	// secretNamePrefix prefixes the secret holding each registry's token.
	secretNamePrefix string
}

// getRegistries is a helper to fetch and unmarshal all registries from the ConfigMap
func (r registryImpl) getRegistries(ctx context.Context) (map[string]Registry, *v1.ConfigMap, error) {
	cm := &v1.ConfigMap{}
	err := r.client.Get(ctx, types.NamespacedName{Namespace: velatypes.DefaultKubeVelaNS, Name: r.cmName}, cm)
	if err != nil {
		return nil, nil, err
	}
	if _, ok := cm.Data[registriesKey]; !ok {
		return nil, nil, NewAddonError("error addon registry configmap registry-key not exist")
	}
	registries := map[string]Registry{}
	if err := json.Unmarshal([]byte(cm.Data[registriesKey]), &registries); err != nil {
		return nil, cm, err
	}
	return registries, cm, nil
}

func (r registryImpl) ListRegistries(ctx context.Context) ([]Registry, error) {
	registries, _, err := r.getRegistries(ctx)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return []Registry{}, nil
		}
		return nil, err
	}

	// getRegistries decodes a map, so iterating it directly hands callers a
	// randomly ordered slice. Callers treat the order as a priority -- the first
	// registry holding an addon wins in FindAddonPackagesDetailFromRegistry, and
	// mergeAddonInfoMaps folds later registries onto earlier ones -- so an addon
	// present in two registries would otherwise resolve differently call to call.
	//
	// By name, because that is the order the data is already stored in: the
	// registries live in a JSON object, which loses insertion order, and
	// encoding/json sorts map keys when AddRegistry marshals it back. So this
	// reproduces the ConfigMap's own order rather than inventing a priority.
	names := make([]string, 0, len(registries))
	for name := range registries {
		names = append(names, name)
	}
	sort.Strings(names)

	res := make([]Registry, 0, len(names))
	for _, name := range names {
		registry := registries[name]
		if err := loadTokenFromSecret(ctx, r.client, &registry); err != nil {
			return nil, err
		}
		res = append(res, registry)
	}
	return res, nil
}

func (r registryImpl) AddRegistry(ctx context.Context, registry Registry) error {
	if err := createOrUpdateTokenSecret(ctx, r.client, &registry, r.secretNamePrefix); err != nil {
		return err
	}

	registries, cm, err := r.getRegistries(ctx)
	if err != nil {
		if apierrors.IsNotFound(err) {
			b, err := json.Marshal(map[string]Registry{
				registry.Name: registry,
			})
			if err != nil {
				return err
			}
			cm := &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      r.cmName,
					Namespace: velatypes.DefaultKubeVelaNS,
				},
				Data: map[string]string{
					registriesKey: string(b),
				},
			}
			return r.client.Create(ctx, cm)
		}
		return err
	}

	registries[registry.Name] = registry
	b, err := json.Marshal(registries)
	if err != nil {
		return err
	}
	cm.Data[registriesKey] = string(b)
	return r.client.Update(ctx, cm)
}

// createOrUpdateTokenSecret will create or update a secret to store registry token
func createOrUpdateTokenSecret(ctx context.Context, cli client.Client, registry *Registry, secretNamePrefix string) error {
	source := registry.GetTokenSource()
	if source == nil {
		return nil
	}
	token := source.GetToken()
	if token == "" {
		return nil
	}
	return migrateInlineTokenToSecret(ctx, cli, registry, source, token, secretNamePrefix)
}

// migrateInlineTokenToSecret will migrate an inline token to a secret.
// It will take the token from the registry object, create/update a secret, and set the secret ref on the registry object.
func migrateInlineTokenToSecret(ctx context.Context, cli client.Client, registry *Registry, source TokenSource, token, secretNamePrefix string) error {
	log := logf.FromContext(ctx)
	secretName := secretNamePrefix + registry.Name
	source.SetTokenSecretRef(secretName)

	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: velatypes.DefaultKubeVelaNS,
		},
		Type: v1.SecretTypeOpaque,
		Data: map[string][]byte{
			"token": []byte(token),
		},
	}

	err := cli.Create(ctx, secret)
	if err != nil {
		if apierrors.IsAlreadyExists(err) {
			existingSecret := &v1.Secret{}
			if err := cli.Get(ctx, types.NamespacedName{Name: secretName, Namespace: velatypes.DefaultKubeVelaNS}, existingSecret); err != nil {
				return err
			}
			existingSecret.Data = secret.Data
			if err := cli.Update(ctx, existingSecret); err != nil {
				return err
			}
			log.Info("Successfully updated secret for addon registry token", "registry", registry.Name, "secret", secretName)
			return nil
		}
		return err
	}
	log.Info("Successfully created secret for addon registry token", "registry", registry.Name, "secret", secretName)
	return nil
}

func (r registryImpl) DeleteRegistry(ctx context.Context, name string) error {
	registries, cm, err := r.getRegistries(ctx)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}

	reg, ok := registries[name]
	if !ok {
		return nil
	}

	if source := reg.GetTokenSource(); source != nil {
		if secretName := source.GetTokenSecretRef(); secretName != "" {
			secret := &v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: velatypes.DefaultKubeVelaNS,
				},
			}
			if err := r.client.Delete(ctx, secret); err != nil && !apierrors.IsNotFound(err) {
				return err
			}
		}
	}

	delete(registries, name)
	b, err := json.Marshal(registries)
	if err != nil {
		return err
	}
	cm.Data[registriesKey] = string(b)
	return r.client.Update(ctx, cm)
}

func (r registryImpl) UpdateRegistry(ctx context.Context, registry Registry) error {
	if err := createOrUpdateTokenSecret(ctx, r.client, &registry, r.secretNamePrefix); err != nil {
		return err
	}
	registries, cm, err := r.getRegistries(ctx)
	if err != nil {
		return err
	}
	if _, ok := registries[registry.Name]; !ok {
		return fmt.Errorf("addon registry %s not exist", registry.Name)
	}
	registries[registry.Name] = registry
	b, err := json.Marshal(registries)
	if err != nil {
		return err
	}
	cm.Data[registriesKey] = string(b)
	return r.client.Update(ctx, cm)
}

func (r registryImpl) GetRegistry(ctx context.Context, name string) (Registry, error) {
	registries, _, err := r.getRegistries(ctx)
	if err != nil {
		return Registry{}, err
	}
	res, ok := registries[name]
	if !ok {
		return res, apierrors.NewNotFound(schema.GroupResource{Group: "addons.kubevela.io", Resource: "Registry"}, name)
	}
	if err := loadTokenFromSecret(ctx, r.client, &res); err != nil {
		return res, err
	}
	return res, nil
}

// loadTokenFromSecret will load token from secret if exists
// and set it to the source of the registry object
func loadTokenFromSecret(ctx context.Context, cli client.Client, registry *Registry) error {
	source := registry.GetTokenSource()
	if source == nil {
		return nil
	}
	secretName := source.GetTokenSecretRef()
	if secretName == "" {
		if source.GetToken() != "" {
			// For backward compatibility, token can be stored in configmap directly.
			// This is not secure, so we print a warning and recommend user to upgrade.
			// The upgrade can be done by editing and saving the addon registry again.
			fmt.Printf("Warning: addon registry %s is using an insecure token stored in ConfigMap. Please edit and save this addon registry again to migrate the token to a secret.\n", registry.Name)
		}
		return nil
	}
	secret := &v1.Secret{}
	if err := cli.Get(ctx, types.NamespacedName{Namespace: velatypes.DefaultKubeVelaNS, Name: secretName}, secret); err != nil {
		if apierrors.IsNotFound(err) {
			// If the secret is not found, we consider the token is empty. Clear
			// TokenSecretRef along with it (SetToken("") does this): otherwise the
			// source is left with an unresolved TokenSecretRef and an empty Token,
			// which HelmSource.validateCredential reads as "a token is configured"
			// even though credential() has nothing to actually send -- passing
			// validation while the transport authenticates with an empty password.
			source.SetToken("")
			return nil
		}
		return err
	}
	source.SetToken(string(secret.Data["token"]))
	return nil
}

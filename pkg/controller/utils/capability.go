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
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-git/go-git/v5"
	gitssh "github.com/go-git/go-git/v5/plumbing/transport/ssh"
	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	commontypes "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/appfile"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/pkg/schema"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	"github.com/oam-dev/kubevela/pkg/utils/terraform"
)

// data types of parameter value
const (
	TerraformVariableString string = "string"
	TerraformVariableNumber string = "number"
	TerraformVariableBool   string = "bool"
	TerraformVariableList   string = "list"
	TerraformVariableTuple  string = "tuple"
	TerraformVariableMap    string = "map"
	TerraformVariableObject string = "object"
	TerraformVariableNull   string = ""
	TerraformVariableAny    string = "any"

	TerraformListTypePrefix   string = "list("
	TerraformTupleTypePrefix  string = "tuple("
	TerraformMapTypePrefix    string = "map("
	TerraformObjectTypePrefix string = "object("
	TerraformSetTypePrefix    string = "set("

	typeTraitDefinition        = "trait"
	typeComponentDefinition    = "component"
	typeWorkflowStepDefinition = "workflowstep"
	typePolicyStepDefinition   = "policy"
)

const (
	// GitCredsKnownHosts is a key in git credentials secret
	GitCredsKnownHosts string = "known_hosts"
)

// ErrNoSectionParameterInCue means there is not parameter section in Cue template of a workload
type ErrNoSectionParameterInCue struct {
	capName string
}

func (e ErrNoSectionParameterInCue) Error() string {
	return fmt.Sprintf("capability %s doesn't contain section `parameter`", e.capName)
}

// CapabilityDefinitionInterface is the interface for Capability (WorkloadDefinition and TraitDefinition)
type CapabilityDefinitionInterface interface {
	GetCapabilityObject(ctx context.Context, k8sClient client.Client, namespace, name string) (*types.Capability, error)
	GetOpenAPISchema(ctx context.Context, k8sClient client.Client, namespace, name string) ([]byte, error)
}

// CapabilityComponentDefinition is the struct for ComponentDefinition
type CapabilityComponentDefinition struct {
	Name                string                      `json:"name"`
	ComponentDefinition v1beta1.ComponentDefinition `json:"componentDefinition"`

	WorkloadType    util.WorkloadType `json:"workloadType"`
	WorkloadDefName string            `json:"workloadDefName"`

	Terraform *commontypes.Terraform `json:"terraform"`
	CapabilityBaseDefinition
}

// NewCapabilityComponentDef will create a CapabilityComponentDefinition
func NewCapabilityComponentDef(componentDefinition *v1beta1.ComponentDefinition) CapabilityComponentDefinition {
	var def CapabilityComponentDefinition
	def.Name = componentDefinition.Name
	if componentDefinition.Spec.Workload.Definition == (commontypes.WorkloadGVK{}) && componentDefinition.Spec.Workload.Type != "" {
		def.WorkloadType = util.ReferWorkload
		def.WorkloadDefName = componentDefinition.Spec.Workload.Type
	}
	if componentDefinition.Spec.Schematic != nil {
		if componentDefinition.Spec.Schematic.Terraform != nil {
			def.WorkloadType = util.TerraformDef
			def.Terraform = componentDefinition.Spec.Schematic.Terraform
		}
	}
	def.ComponentDefinition = *componentDefinition.DeepCopy()
	return def
}

// GetOpenAPISchema gets OpenAPI v3 schema by WorkloadDefinition name
func (def *CapabilityComponentDefinition) GetOpenAPISchema(ctx context.Context, name string) ([]byte, error) {
	capability, err := appfile.ConvertTemplateJSON2Object(name, def.ComponentDefinition.Spec.Extension, def.ComponentDefinition.Spec.Schematic)
	if err != nil {
		return nil, fmt.Errorf("failed to convert ComponentDefinition to Capability Object")
	}
	return getOpenAPISchema(ctx, capability)
}

// GetOpenAPISchemaFromTerraformComponentDefinition gets OpenAPI v3 schema by WorkloadDefinition name
func GetOpenAPISchemaFromTerraformComponentDefinition(configuration string) ([]byte, error) {
	schemas := make(map[string]*openapi3.Schema)
	var required []string
	variables, _, err := common.ParseTerraformVariables(configuration)
	if err != nil {
		return nil, errors.Wrap(err, "failed to generate capability properties")
	}
	for k, v := range variables {
		var schema *openapi3.Schema
		switch v.Type {
		case TerraformVariableString:
			schema = openapi3.NewStringSchema()
		case TerraformVariableNumber:
			schema = openapi3.NewFloat64Schema()
		case TerraformVariableBool:
			schema = openapi3.NewBoolSchema()
		case TerraformVariableList, TerraformVariableTuple:
			schema = openapi3.NewArraySchema()
		case TerraformVariableMap, TerraformVariableObject:
			schema = openapi3.NewObjectSchema()
		case TerraformVariableAny:
			switch v.Default.(type) {
			case []interface{}:
				schema = openapi3.NewArraySchema()
			case map[string]interface{}:
				schema = openapi3.NewObjectSchema()
			}
		case TerraformVariableNull:
			switch v.Default.(type) {
			case nil, string:
				schema = openapi3.NewStringSchema()
			case []interface{}:
				schema = openapi3.NewArraySchema()
			case map[string]interface{}:
				schema = openapi3.NewObjectSchema()
			case int, float64:
				schema = openapi3.NewFloat64Schema()
			default:
				return nil, fmt.Errorf("null type variable is NOT supported, please specify a type for the variable: %s", v.Name)
			}
		}

		// To identify unusual list type
		if schema == nil {
			switch {
			case strings.HasPrefix(v.Type, TerraformListTypePrefix) || strings.HasPrefix(v.Type, TerraformTupleTypePrefix) ||
				strings.HasPrefix(v.Type, TerraformSetTypePrefix):
				schema = openapi3.NewArraySchema()
			case strings.HasPrefix(v.Type, TerraformMapTypePrefix) || strings.HasPrefix(v.Type, TerraformObjectTypePrefix):
				schema = openapi3.NewObjectSchema()
			default:
				return nil, fmt.Errorf("the type `%s` of variable %s is NOT supported", v.Type, v.Name)
			}
		}
		schema.Title = k
		if v.Required {
			required = append(required, k)
		}
		if v.Default != nil {
			schema.Default = v.Default
		}
		schema.Description = v.Description
		schemas[v.Name] = schema
	}

	otherProperties := parseOtherProperties4TerraformDefinition()
	for k, v := range otherProperties {
		schemas[k] = v
	}

	return generateJSONSchemaWithRequiredProperty(schemas, required)
}

// GetTerraformConfigurationFromRemote gets Terraform Configuration(HCL)
func GetTerraformConfigurationFromRemote(name, remoteURL, remotePath string, sshPublicKey *gitssh.PublicKeys) (string, error) {
	if err := validateModuleName(name); err != nil {
		return "", err
	}
	userHome, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	cachePath := filepath.Join(userHome, ".vela", "terraform", name)

	// Reuse the cache only when it is populated AND was cloned from the same URL.
	// Otherwise (empty, a changed URL, or a missing marker) re-clone, so a
	// definition repointed at a different repository does not keep serving the old
	// tree. See GHSA-fmgp-q6jx-gg3x.
	if !cacheMatchesRemote(cachePath, remoteURL) {
		_ = os.RemoveAll(cachePath)
		klog.InfoS("cloning remote Terraform module", "module", name, "url", remoteURL, "cache", cachePath)
		if err = cloneTerraformModule(cachePath, remoteURL, sshPublicKey); err != nil {
			// Do not leave a partial or oversized clone behind for the next reconcile.
			klog.ErrorS(err, "failed to clone remote Terraform module", "module", name, "url", remoteURL)
			_ = os.RemoveAll(cachePath)
			return "", err
		}
		recordCacheRemote(cachePath, remoteURL)
	}
	sshKnownHostsPath := os.Getenv("SSH_KNOWN_HOSTS")
	_ = os.Remove(sshKnownHostsPath)

	conf, err := readTerraformConfigFromDir(cachePath, remotePath)
	if err != nil {
		// Drop the cached clone on any rejection or read failure so a corrected or
		// replaced repository is re-cloned next time, rather than the controller
		// re-reading a poisoned or stale tree forever. Wiping on a transient read
		// error only costs one bounded re-clone, which beats risking a stuck poison.
		klog.InfoS("evicting Terraform module cache after a failed read", "module", name, "cache", cachePath, "err", err.Error())
		_ = os.RemoveAll(cachePath)
		return "", err
	}
	return conf, nil
}

// cacheRemoteMarkerSuffix names the sibling file that records which remote URL a
// cached Terraform module was cloned from, used to detect a changed URL.
const cacheRemoteMarkerSuffix = ".remote-url"

// validateModuleName rejects names that could steer the cache path (and the
// os.RemoveAll that cleans it) outside the module cache directory. Callers pass a
// Kubernetes object name, but the check keeps that guarantee local and explicit.
func validateModuleName(name string) error {
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, `/\`) {
		return errors.Errorf("invalid Terraform module name %q", name)
	}
	return nil
}

// cacheMatchesRemote reports whether cachePath holds a populated clone recorded
// as coming from remoteURL.
func cacheMatchesRemote(cachePath, remoteURL string) bool {
	entities, err := os.ReadDir(cachePath)
	if err != nil || len(entities) == 0 {
		return false
	}
	recorded, err := os.ReadFile(cachePath + cacheRemoteMarkerSuffix)
	if err != nil {
		return false
	}
	return string(recorded) == remoteURL
}

// recordCacheRemote records the remote URL a freshly cloned module came from.
func recordCacheRemote(cachePath, remoteURL string) {
	if err := os.WriteFile(cachePath+cacheRemoteMarkerSuffix, []byte(remoteURL), 0600); err != nil {
		klog.ErrorS(err, "failed to record Terraform module remote URL", "cache", cachePath)
	}
}

const (
	// cloneTimeout bounds the network fetch of a remote Terraform module clone, so a
	// slow or oversized attacker-supplied repository cannot hang the controller. It
	// covers the git transport only; the worktree checkout that follows is not bound
	// by this deadline.
	cloneTimeout = 2 * time.Minute
	// maxCloneBytes and maxCloneEntries cap the on-disk footprint of a cloned remote
	// Terraform module (modules are small). They run AFTER the clone completes, so
	// they bound the RETAINED clone (an oversized clone is rejected and removed by
	// the caller) rather than peak transient disk during transfer/checkout; Depth:1
	// keeps that transient footprint to a single shallow commit. Fully bounding peak
	// transfer disk (NoCheckout plus object-store reads, or an instrumented
	// filesystem) is a follow-up. See GHSA-fmgp-q6jx-gg3x.
	maxCloneBytes int64 = 64 << 20 // 64 MiB
	// maxCloneEntries counts every filesystem entry (files, directories, and
	// symlinks), so a tree of many empty directories or symlinks cannot bypass the
	// bound and exhaust inodes.
	maxCloneEntries = 50000
)

// cloneTerraformModule shallow-clones remoteURL into cachePath under a fetch
// timeout, then rejects an oversized clone. Depth:1 drops history, the timeout
// bounds the network fetch, and the post-clone caps reject a clone whose retained
// size or entry count is too large (the caller removes the rejected clone).
func cloneTerraformModule(cachePath, remoteURL string, sshPublicKey *gitssh.PublicKeys) error {
	ctx, cancel := context.WithTimeout(context.Background(), cloneTimeout)
	defer cancel()

	cloneOptions := &git.CloneOptions{
		URL:      remoteURL,
		Progress: os.Stdout,
		Depth:    1,
	}
	if sshPublicKey != nil {
		cloneOptions.Auth = sshPublicKey
	}
	if _, err := git.PlainCloneContext(ctx, cachePath, false, cloneOptions); err != nil {
		return errors.Wrap(err, "failed to clone remote Terraform configuration")
	}
	return ensureDirWithinLimits(cachePath, maxCloneBytes, maxCloneEntries)
}

// ensureDirWithinLimits returns an error as soon as the entries under root exceed
// maxEntries in count or the regular files exceed maxBytes in total size. Every
// entry (regular files, directories, and symlinks) counts toward maxEntries, so a
// tree of many directories or symlinks cannot slip past the bound and exhaust
// inodes; only regular files contribute to the byte total. It counts everything
// under root (including .git, by design) and does not follow symlinks. The caps
// bound the retained clone, not peak disk during the clone (see maxCloneBytes).
func ensureDirWithinLimits(root string, maxBytes int64, maxEntries int) error {
	var (
		total   int64
		entries int
	)
	return filepath.WalkDir(root, func(_ string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		entries++
		if entries > maxEntries {
			return errors.Errorf("remote Terraform repository exceeds the maximum allowed entry count of %d", maxEntries)
		}
		if !d.Type().IsRegular() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		total += info.Size()
		if total > maxBytes {
			return errors.Errorf("remote Terraform repository exceeds the maximum allowed size of %d bytes", maxBytes)
		}
		return nil
	})
}

// maxTerraformConfigBytes caps how much of a remote Terraform configuration
// file the loader will read. HCL configurations are small, so the cap stops a
// malicious or compromised repository from steering the read at an unbounded
// source (for example a variables.tf symlinked to /dev/zero) and OOM-killing
// the controller. See GHSA-fmgp-q6jx-gg3x.
const maxTerraformConfigBytes int64 = 1 << 20 // 1 MiB

// errTerraformConfigTooLarge builds the error returned when a candidate
// configuration file exceeds maxTerraformConfigBytes.
func errTerraformConfigTooLarge(path string, maxSize int64) error {
	return errors.Errorf("refusing to read Terraform configuration %q: exceeds the maximum allowed size of %d bytes", path, maxSize)
}

// readTerraformConfigFromDir reads variables.tf, or main.tf as a fallback, from
// the cloned remote repository at cacheRoot/remotePath. It refuses any candidate
// that is not a regular file contained within cacheRoot (defeating symlink and
// path-traversal escapes) and any file larger than maxTerraformConfigBytes.
func readTerraformConfigFromDir(cacheRoot, remotePath string) (string, error) {
	baseDir := filepath.Join(cacheRoot, remotePath)
	for _, name := range []string{"variables.tf", "main.tf"} {
		content, found, err := readContainedRegularFile(cacheRoot, filepath.Join(baseDir, name), maxTerraformConfigBytes)
		if err != nil {
			return "", err
		}
		if found {
			return content, nil
		}
	}
	return "", errors.New("failed to find main.tf or variables.tf in Terraform configurations of the remote repository")
}

// readContainedRegularFile reads path when it is a regular file whose real
// location stays inside root and whose size is within maxSize. found is false
// only when path does not exist, so callers can try a fallback name; any other
// problem (a symlink or traversal escape, an irregular file, or an oversize
// file) is a hard error rather than a silent fallback.
func readContainedRegularFile(root, path string, maxSize int64) (content string, found bool, err error) {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", false, nil
		}
		return "", false, errors.Wrap(err, "failed to stat Terraform configuration")
	}
	// Reject any symlink at the target outright, even one that resolves inside
	// the cache. This is intentionally stricter than the containment check
	// below: variables.tf/main.tf are expected to be regular files, so a blanket
	// rejection is the simplest defense against the GHSA-fmgp-q6jx-gg3x
	// symlink-to-/dev/zero vector. The containment check still guards a symlinked
	// parent directory whose leaf is itself a regular file.
	if info.Mode()&os.ModeSymlink != 0 {
		return "", true, errors.Errorf("refusing to read symlinked Terraform configuration %q", path)
	}

	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", true, errors.Wrap(err, "failed to resolve Terraform cache directory")
	}
	realPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", true, errors.Wrap(err, "failed to resolve Terraform configuration path")
	}
	if realPath != realRoot && !strings.HasPrefix(realPath, realRoot+string(os.PathSeparator)) {
		return "", true, errors.Errorf("refusing to read Terraform configuration outside the cache directory: %q", path)
	}

	if !info.Mode().IsRegular() {
		return "", true, errors.Errorf("refusing to read non-regular Terraform configuration %q", path)
	}
	if info.Size() > maxSize {
		return "", true, errTerraformConfigTooLarge(path, maxSize)
	}

	f, err := os.Open(filepath.Clean(path))
	if err != nil {
		return "", true, errors.Wrap(err, "failed to read Terraform configuration")
	}
	defer func() { _ = f.Close() }()

	buf, err := io.ReadAll(io.LimitReader(f, maxSize+1))
	if err != nil {
		return "", true, errors.Wrap(err, "failed to read Terraform configuration")
	}
	if int64(len(buf)) > maxSize {
		return "", true, errTerraformConfigTooLarge(path, maxSize)
	}
	return string(buf), true, nil
}

func parseOtherProperties4TerraformDefinition() map[string]*openapi3.Schema {
	otherProperties := make(map[string]*openapi3.Schema)

	// 1. writeConnectionSecretToRef
	secretName := openapi3.NewStringSchema()
	secretName.Title = "name"
	secretName.Description = terraform.TerraformSecretNameDescription

	secretNamespace := openapi3.NewStringSchema()
	secretNamespace.Title = "namespace"
	secretNamespace.Description = terraform.TerraformSecretNamespaceDescription

	secret := openapi3.NewObjectSchema()
	secret.Title = terraform.TerraformWriteConnectionSecretToRefName
	secret.Description = terraform.TerraformWriteConnectionSecretToRefDescription
	secret.Properties = openapi3.Schemas{
		"name":      &openapi3.SchemaRef{Value: secretName},
		"namespace": &openapi3.SchemaRef{Value: secretNamespace},
	}
	secret.Required = []string{"name"}

	otherProperties[terraform.TerraformWriteConnectionSecretToRefName] = secret

	// 2. providerRef
	providerName := openapi3.NewStringSchema()
	providerName.Title = "name"
	providerName.Description = "The name of the Terraform Cloud provider"

	providerNamespace := openapi3.NewStringSchema()
	providerNamespace.Title = "namespace"
	providerNamespace.Default = "default"
	providerNamespace.Description = "The namespace of the Terraform Cloud provider"

	var providerRefName = "providerRef"
	provider := openapi3.NewObjectSchema()
	provider.Title = providerRefName
	provider.Description = "specifies the Provider"
	provider.Properties = openapi3.Schemas{
		"name":      &openapi3.SchemaRef{Value: providerName},
		"namespace": &openapi3.SchemaRef{Value: providerNamespace},
	}
	provider.Required = []string{"name"}

	otherProperties[providerRefName] = provider

	// 3. deleteResource
	var deleteResourceName = "deleteResource"
	deleteResource := openapi3.NewBoolSchema()
	deleteResource.Title = deleteResourceName
	deleteResource.Description = "DeleteResource will determine whether provisioned cloud resources will be deleted when application is deleted"
	deleteResource.Default = true
	otherProperties[deleteResourceName] = deleteResource

	// 4. region
	var regionName = "region"
	region := openapi3.NewStringSchema()
	region.Title = regionName
	region.Description = "Region is cloud provider's region. It will override providerRef"
	otherProperties[regionName] = region

	return otherProperties

}

func generateJSONSchemaWithRequiredProperty(schemas map[string]*openapi3.Schema, required []string) ([]byte, error) {
	s := openapi3.NewObjectSchema().WithProperties(schemas)
	if len(required) > 0 {
		s.Required = required
	}
	b, err := s.MarshalJSON()
	if err != nil {
		return nil, errors.Wrap(err, "cannot marshal generated schema into json")
	}
	return b, nil
}

// GetGitSSHPublicKey gets a kubernetes secret containing the SSH private key based on gitCredentialsSecretReference parameters for component and trait definition
func GetGitSSHPublicKey(ctx context.Context, k8sClient client.Client, gitCredentialsSecretReference *v1.SecretReference) (*gitssh.PublicKeys, error) {
	gitCredentialsSecretName := gitCredentialsSecretReference.Name
	gitCredentialsSecretNamespace := gitCredentialsSecretReference.Namespace
	gitCredentialsNamespacedName := k8stypes.NamespacedName{Namespace: gitCredentialsSecretNamespace, Name: gitCredentialsSecretName}

	secret := &v1.Secret{}
	err := k8sClient.Get(ctx, gitCredentialsNamespacedName, secret)
	if err != nil {
		return nil, fmt.Errorf("failed to  get git credentials secret: %w", err)
	}
	needSecretKeys := []string{GitCredsKnownHosts, v1.SSHAuthPrivateKey}
	for _, key := range needSecretKeys {
		if _, ok := secret.Data[key]; !ok {
			err := errors.Errorf("'%s' not in git credentials secret", key)
			return nil, err
		}
	}

	klog.InfoS("Reconcile gitCredentialsSecretReference", "gitCredentialsSecretReference", klog.KRef(gitCredentialsSecretNamespace, gitCredentialsSecretName))

	sshPrivateKey := secret.Data[v1.SSHAuthPrivateKey]
	publicKey, err := gitssh.NewPublicKeys("git", sshPrivateKey, "")
	if err != nil {
		return nil, fmt.Errorf("failed to generate public key from private key: %w", err)
	}
	sshKnownHosts := secret.Data[GitCredsKnownHosts]
	sshDir := filepath.Join(os.TempDir(), ".ssh")
	sshKnownHostsPath := filepath.Join(sshDir, GitCredsKnownHosts)
	_ = os.Mkdir(sshDir, 0700)
	err = os.WriteFile(sshKnownHostsPath, sshKnownHosts, 0600)
	if err != nil {
		return nil, fmt.Errorf("failed to write known hosts into file: %w", err)
	}
	_ = os.Setenv("SSH_KNOWN_HOSTS", sshKnownHostsPath)
	return publicKey, nil
}

// StoreOpenAPISchema stores OpenAPI v3 schema in ConfigMap from WorkloadDefinition
func (def *CapabilityComponentDefinition) StoreOpenAPISchema(ctx context.Context, k8sClient client.Client, namespace, name, revName string) (string, error) {
	var jsonSchema []byte
	var err error
	switch def.WorkloadType {
	case util.TerraformDef:
		if def.Terraform == nil {
			return "", fmt.Errorf("no Configuration is set in Terraform specification: %s", def.Name)
		}
		configuration := def.Terraform.Configuration
		if def.Terraform.Type == "remote" {
			var publicKey *gitssh.PublicKeys
			publicKey = nil
			if def.Terraform.GitCredentialsSecretReference != nil {
				gitCredentialsSecretReference := def.Terraform.GitCredentialsSecretReference
				publicKey, err = GetGitSSHPublicKey(ctx, k8sClient, gitCredentialsSecretReference)
				if err != nil {
					return "", fmt.Errorf("issue with gitCredentialsSecretReference %s/%s: %w", gitCredentialsSecretReference.Namespace, gitCredentialsSecretReference.Name, err)
				}
			}
			configuration, err = GetTerraformConfigurationFromRemote(def.Name, def.Terraform.Configuration, def.Terraform.Path, publicKey)
			if err != nil {
				return "", fmt.Errorf("cannot get Terraform configuration %s from remote: %w", def.Name, err)
			}
		}
		jsonSchema, err = GetOpenAPISchemaFromTerraformComponentDefinition(configuration)
	default:
		jsonSchema, err = def.GetOpenAPISchema(ctx, name)
	}
	if err != nil {
		return "", fmt.Errorf("failed to generate OpenAPI v3 JSON schema for capability %s: %w", def.Name, err)
	}
	componentDefinition := def.ComponentDefinition
	ownerReference := []metav1.OwnerReference{{
		APIVersion:         componentDefinition.APIVersion,
		Kind:               componentDefinition.Kind,
		Name:               componentDefinition.Name,
		UID:                componentDefinition.GetUID(),
		Controller:         ptr.To(true),
		BlockOwnerDeletion: ptr.To(true),
	}}
	cmName, err := def.CreateOrUpdateConfigMap(ctx, k8sClient, namespace, componentDefinition.Name, typeComponentDefinition, componentDefinition.Labels, nil, jsonSchema, ownerReference)
	if err != nil {
		return cmName, err
	}

	// Create a configmap to store parameter for each definitionRevision
	defRev := new(v1beta1.DefinitionRevision)
	if err = k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: revName}, defRev); err != nil {
		return "", err
	}
	ownerReference = []metav1.OwnerReference{{
		APIVersion:         defRev.APIVersion,
		Kind:               defRev.Kind,
		Name:               defRev.Name,
		UID:                defRev.GetUID(),
		Controller:         ptr.To(true),
		BlockOwnerDeletion: ptr.To(true),
	}}
	_, err = def.CreateOrUpdateConfigMap(ctx, k8sClient, namespace, revName, typeComponentDefinition, defRev.Spec.ComponentDefinition.Labels, nil, jsonSchema, ownerReference)
	if err != nil {
		return cmName, err
	}
	return cmName, nil
}

// CapabilityTraitDefinition is the Capability struct for TraitDefinition
type CapabilityTraitDefinition struct {
	Name            string                  `json:"name"`
	TraitDefinition v1beta1.TraitDefinition `json:"traitDefinition"`

	DefCategoryType util.WorkloadType `json:"defCategoryType"`

	CapabilityBaseDefinition
}

// NewCapabilityTraitDef will create a CapabilityTraitDefinition
func NewCapabilityTraitDef(traitdefinition *v1beta1.TraitDefinition) CapabilityTraitDefinition {
	var def CapabilityTraitDefinition
	def.Name = traitdefinition.Name //  or def.Name = req.NamespacedName.Name
	def.TraitDefinition = *traitdefinition.DeepCopy()
	return def
}

// GetOpenAPISchema gets OpenAPI v3 schema by TraitDefinition name
func (def *CapabilityTraitDefinition) GetOpenAPISchema(ctx context.Context, name string) ([]byte, error) {
	capability, err := appfile.ConvertTemplateJSON2Object(name, def.TraitDefinition.Spec.Extension, def.TraitDefinition.Spec.Schematic)
	if err != nil {
		return nil, fmt.Errorf("failed to convert WorkloadDefinition to Capability Object")
	}
	return getOpenAPISchema(ctx, capability)
}

// StoreOpenAPISchema stores OpenAPI v3 schema from TraitDefinition in ConfigMap
func (def *CapabilityTraitDefinition) StoreOpenAPISchema(ctx context.Context, k8sClient client.Client, namespace, name string, revName string) (string, error) {
	jsonSchema, err := def.GetOpenAPISchema(ctx, name)
	if err != nil {
		return "", fmt.Errorf("failed to generate OpenAPI v3 JSON schema for capability %s: %w", def.Name, err)
	}

	traitDefinition := def.TraitDefinition
	ownerReference := []metav1.OwnerReference{{
		APIVersion:         traitDefinition.APIVersion,
		Kind:               traitDefinition.Kind,
		Name:               traitDefinition.Name,
		UID:                traitDefinition.GetUID(),
		Controller:         ptr.To(true),
		BlockOwnerDeletion: ptr.To(true),
	}}
	cmName, err := def.CreateOrUpdateConfigMap(ctx, k8sClient, namespace, traitDefinition.Name, typeTraitDefinition, traitDefinition.Labels, traitDefinition.Spec.AppliesToWorkloads, jsonSchema, ownerReference)
	if err != nil {
		return cmName, err
	}

	// Create a configmap to store parameter for each definitionRevision
	defRev := new(v1beta1.DefinitionRevision)
	if err = k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: revName}, defRev); err != nil {
		return "", err
	}
	ownerReference = []metav1.OwnerReference{{
		APIVersion:         defRev.APIVersion,
		Kind:               defRev.Kind,
		Name:               defRev.Name,
		UID:                defRev.GetUID(),
		Controller:         ptr.To(true),
		BlockOwnerDeletion: ptr.To(true),
	}}
	_, err = def.CreateOrUpdateConfigMap(ctx, k8sClient, namespace, revName, typeTraitDefinition, defRev.Spec.TraitDefinition.Labels, defRev.Spec.TraitDefinition.Spec.AppliesToWorkloads, jsonSchema, ownerReference)
	if err != nil {
		return cmName, err
	}
	return cmName, nil
}

// CapabilityStepDefinition is the Capability struct for WorkflowStepDefinition
type CapabilityStepDefinition struct {
	Name           string                         `json:"name"`
	StepDefinition v1beta1.WorkflowStepDefinition `json:"stepDefinition"`

	CapabilityBaseDefinition
}

// NewCapabilityStepDef will create a CapabilityStepDefinition
func NewCapabilityStepDef(stepdefinition *v1beta1.WorkflowStepDefinition) CapabilityStepDefinition {
	var def CapabilityStepDefinition
	def.Name = stepdefinition.Name
	def.StepDefinition = *stepdefinition.DeepCopy()
	return def
}

// GetOpenAPISchema gets OpenAPI v3 schema by StepDefinition name
func (def *CapabilityStepDefinition) GetOpenAPISchema(ctx context.Context, name string) ([]byte, error) {
	capability, err := appfile.ConvertTemplateJSON2Object(name, nil, def.StepDefinition.Spec.Schematic)
	if err != nil {
		return nil, fmt.Errorf("failed to convert WorkflowStepDefinition to Capability Object")
	}
	return getOpenAPISchema(ctx, capability)
}

// StoreOpenAPISchema stores OpenAPI v3 schema from StepDefinition in ConfigMap
func (def *CapabilityStepDefinition) StoreOpenAPISchema(ctx context.Context, k8sClient client.Client, namespace, name string, revName string) (string, error) {
	var jsonSchema []byte
	var err error

	jsonSchema, err = def.GetOpenAPISchema(ctx, name)
	if err != nil {
		return "", fmt.Errorf("failed to generate OpenAPI v3 JSON schema for capability %s: %w", def.Name, err)
	}

	stepDefinition := def.StepDefinition
	ownerReference := []metav1.OwnerReference{{
		APIVersion:         stepDefinition.APIVersion,
		Kind:               stepDefinition.Kind,
		Name:               stepDefinition.Name,
		UID:                stepDefinition.GetUID(),
		Controller:         ptr.To(true),
		BlockOwnerDeletion: ptr.To(true),
	}}
	cmName, err := def.CreateOrUpdateConfigMap(ctx, k8sClient, namespace, stepDefinition.Name, typeWorkflowStepDefinition, stepDefinition.Labels, nil, jsonSchema, ownerReference)
	if err != nil {
		return cmName, err
	}

	// Create a configmap to store parameter for each definitionRevision
	defRev := new(v1beta1.DefinitionRevision)
	if err = k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: revName}, defRev); err != nil {
		return "", err
	}
	ownerReference = []metav1.OwnerReference{{
		APIVersion:         defRev.APIVersion,
		Kind:               defRev.Kind,
		Name:               defRev.Name,
		UID:                defRev.GetUID(),
		Controller:         ptr.To(true),
		BlockOwnerDeletion: ptr.To(true),
	}}
	_, err = def.CreateOrUpdateConfigMap(ctx, k8sClient, namespace, revName, typeWorkflowStepDefinition, defRev.Spec.WorkflowStepDefinition.Labels, nil, jsonSchema, ownerReference)
	if err != nil {
		return cmName, err
	}
	return cmName, nil
}

// CapabilityPolicyDefinition is the Capability struct for PolicyDefinition
type CapabilityPolicyDefinition struct {
	Name             string                   `json:"name"`
	PolicyDefinition v1beta1.PolicyDefinition `json:"policyDefinition"`

	CapabilityBaseDefinition
}

// NewCapabilityPolicyDef will create a CapabilityPolicyDefinition
func NewCapabilityPolicyDef(policydefinition *v1beta1.PolicyDefinition) CapabilityPolicyDefinition {
	var def CapabilityPolicyDefinition
	def.Name = policydefinition.Name
	def.PolicyDefinition = *policydefinition.DeepCopy()
	return def
}

// GetOpenAPISchema gets OpenAPI v3 schema by StepDefinition name
func (def *CapabilityPolicyDefinition) GetOpenAPISchema(ctx context.Context, name string) ([]byte, error) {
	capability, err := appfile.ConvertTemplateJSON2Object(name, nil, def.PolicyDefinition.Spec.Schematic)
	if err != nil {
		return nil, fmt.Errorf("failed to convert WorkflowStepDefinition to Capability Object")
	}
	return getOpenAPISchema(ctx, capability)
}

// StoreOpenAPISchema stores OpenAPI v3 schema from StepDefinition in ConfigMap
func (def *CapabilityPolicyDefinition) StoreOpenAPISchema(ctx context.Context, k8sClient client.Client, namespace, name, revName string) (string, error) {
	var jsonSchema []byte
	var err error

	jsonSchema, err = def.GetOpenAPISchema(ctx, name)
	if err != nil {
		return "", fmt.Errorf("failed to generate OpenAPI v3 JSON schema for capability %s: %w", def.Name, err)
	}

	policyDefinition := def.PolicyDefinition
	ownerReference := []metav1.OwnerReference{{
		APIVersion:         policyDefinition.APIVersion,
		Kind:               policyDefinition.Kind,
		Name:               policyDefinition.Name,
		UID:                policyDefinition.GetUID(),
		Controller:         ptr.To(true),
		BlockOwnerDeletion: ptr.To(true),
	}}
	cmName, err := def.CreateOrUpdateConfigMap(ctx, k8sClient, namespace, policyDefinition.Name, typePolicyStepDefinition, policyDefinition.Labels, nil, jsonSchema, ownerReference)
	if err != nil {
		return cmName, err
	}

	// Create a configmap to store parameter for each definitionRevision
	defRev := new(v1beta1.DefinitionRevision)
	if err = k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: revName}, defRev); err != nil {
		return "", err
	}
	ownerReference = []metav1.OwnerReference{{
		APIVersion:         defRev.APIVersion,
		Kind:               defRev.Kind,
		Name:               defRev.Name,
		UID:                defRev.GetUID(),
		Controller:         ptr.To(true),
		BlockOwnerDeletion: ptr.To(true),
	}}
	_, err = def.CreateOrUpdateConfigMap(ctx, k8sClient, namespace, revName, typePolicyStepDefinition, defRev.Spec.PolicyDefinition.Labels, nil, jsonSchema, ownerReference)
	if err != nil {
		return cmName, err
	}
	return cmName, nil
}

// CapabilityBaseDefinition is the base struct for CapabilityWorkloadDefinition and CapabilityTraitDefinition
type CapabilityBaseDefinition struct {
}

// CreateOrUpdateConfigMap creates ConfigMap to store OpenAPI v3 schema or or updates data in ConfigMap
func (def *CapabilityBaseDefinition) CreateOrUpdateConfigMap(ctx context.Context, k8sClient client.Client, namespace,
	definitionName, definitionType string, labels map[string]string, appliedWorkloads []string, jsonSchema []byte, ownerReferences []metav1.OwnerReference) (string, error) {
	cmName := fmt.Sprintf("%s-%s%s", definitionType, types.CapabilityConfigMapNamePrefix, definitionName)
	var cm v1.ConfigMap
	var data = map[string]string{
		types.OpenapiV3JSONSchema: string(jsonSchema),
	}
	if labels == nil {
		labels = make(map[string]string)
	}
	labels[types.LabelDefinition] = "schema"
	labels[types.LabelDefinitionName] = definitionName
	annotations := make(map[string]string)
	if appliedWorkloads != nil {
		annotations[types.AnnoDefinitionAppliedWorkloads] = strings.Join(appliedWorkloads, ",")
	}

	// No need to check the existence of namespace, if it doesn't exist, API server will return the error message
	// before it's to be reconciled by ComponentDefinition/TraitDefinition controller.
	err := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: cmName}, &cm)
	if err != nil && apierrors.IsNotFound(err) {
		cm = v1.ConfigMap{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "v1",
				Kind:       "ConfigMap",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:            cmName,
				Namespace:       namespace,
				OwnerReferences: ownerReferences,
				Labels:          labels,
				Annotations:     annotations,
			},
			Data: data,
		}
		err = k8sClient.Create(ctx, &cm)
		if err != nil {
			return cmName, fmt.Errorf(util.ErrUpdateCapabilityInConfigMap, definitionName, err)
		}
		klog.InfoS("Successfully stored Capability Schema in ConfigMap", "configMap", klog.KRef(namespace, cmName))
		return cmName, nil
	}

	cm.Data = data
	cm.Labels = labels
	cm.Annotations = annotations
	if err = k8sClient.Update(ctx, &cm); err != nil {
		return cmName, fmt.Errorf(util.ErrUpdateCapabilityInConfigMap, definitionName, err)
	}
	klog.InfoS("Successfully update Capability Schema in ConfigMap", "configMap", klog.KRef(namespace, cmName))
	return cmName, nil
}

// getOpenAPISchema is the main function for GetDefinition API
func getOpenAPISchema(ctx context.Context, capability types.Capability) ([]byte, error) {
	s, err := schema.ParsePropertiesToSchema(ctx, capability.CueTemplate)
	if err != nil {
		return nil, err
	}
	klog.Infof("parsed %d properties by %s/%s", len(s.Properties), capability.Type, capability.Name)
	parameter, err := s.MarshalJSON()
	if err != nil {
		return nil, err
	}
	return parameter, nil
}

/*
Copyright 2022 The KubeVela Authors.

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

package service

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/oam-dev/kubevela/pkg/apiserver/utils/bcode"
	"github.com/oam-dev/kubevela/pkg/config"
	"github.com/oam-dev/kubevela/pkg/utils/registries"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/remote/transport"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/types"
	v1 "github.com/oam-dev/kubevela/pkg/apiserver/interfaces/api/dto/v1"
)

// True -
const True = "true"

// NewImageService create a image service instance
func NewImageService() ImageService {
	return &imageImpl{}
}

// ImageService the image service provide some handler functions about the docker image
type ImageService interface {
	ListImageRepos(ctx context.Context, project string) ([]v1.ImageRegistry, error)
	GetImageInfo(ctx context.Context, project, secretName, imageName string) v1.ImageInfo
	VerifyImageRepoSecret(ctx context.Context, project string, req v1.CreateConfigRequest) (*v1.Config, error)
	GetRepositoryTags(ctx context.Context, project, secretName, repository string) (*registries.RepositoryTags, error)
}

type imageImpl struct {
	K8sClient      client.Client             `inject:"kubeClient"`
	ConfigService  ConfigService             `inject:""`
	ProjectService ProjectService            `inject:""`
	Factory        config.Factory            `inject:"configFactory"`
	registryHelper registries.RegistryHelper `inject:"registryHelper"`
}

// ListImageRepos list the image repositories via user configuration
func (i *imageImpl) ListImageRepos(ctx context.Context, project string) ([]v1.ImageRegistry, error) {
	configs, err := i.ConfigService.ListConfigs(ctx, project, types.ImageRegistry, true)
	if err != nil {
		return nil, err
	}
	var repos []v1.ImageRegistry
	for _, item := range configs {
		if item.Properties != nil {
			registry, ok := item.Properties["registry"].(string)
			if ok {
				repos = append(repos, v1.ImageRegistry{
					Name:       item.Name,
					SecretName: item.Name,
					Domain:     registry,
					Secret:     item.Secret,
				})
			}
		}
	}
	return repos, nil
}

// GetImageInfo get the image info from image registry
func (i *imageImpl) GetImageInfo(ctx context.Context, project, secretName, imageName string) v1.ImageInfo {
	var imageInfo = v1.ImageInfo{
		Name: imageName,
	}
	ref, err := name.ParseReference(imageName)
	if err != nil {
		imageInfo.Message = "The image name is invalid."
		return imageInfo
	}
	registryDomain := ref.Context().RegistryStr()
	imageInfo.Registry = registryDomain

	registries, err := i.ListImageRepos(ctx, project)
	if err != nil {
		klog.Warningf("fail to list the image registries:%s", err.Error())
		imageInfo.Message = "There is no registry."
		return imageInfo
	}
	var selectRegistry []v1.ImageRegistry
	var selectRegistryNames []string
	// get info with specified secret
	if secretName != "" {
		for i, registry := range registries {
			if secretName == registry.SecretName {
				selectRegistry = append(selectRegistry, registries[i])
				selectRegistryNames = append(selectRegistryNames, registry.Name)
				break
			}
		}
	}

	// get info with the secret which match the registry domain
	if selectRegistry == nil {
		for i, registry := range registries {
			if registry.Domain == registryDomain {
				selectRegistry = append(selectRegistry, registries[i])
				selectRegistryNames = append(selectRegistryNames, registry.Name)
			}
		}
	}
	var username, password string
	var insecure = false
	var useHTTP = false
	imageInfo.SecretNames = selectRegistryNames
	for _, registry := range selectRegistry {
		if registry.Secret != nil {
			insecure, useHTTP, username, password = getAccountFromSecret(*registry.Secret, registryDomain)
			break
		}
	}
	err = getImageInfo(imageName, insecure, useHTTP, username, password, &imageInfo)
	if err != nil {
		imageInfo.Message = fmt.Sprintf("Fail to get the image info:%s", err.Error())
	}
	return imageInfo
}

func (i *imageImpl) VerifyImageRepoSecret(ctx context.Context, project string, req v1.CreateConfigRequest) (*v1.Config, error) {
	ns := types.DefaultKubeVelaNS
	if project != "" {
		pro, err := i.ProjectService.GetProject(ctx, project)
		if err != nil {
			return nil, err
		}
		ns = pro.GetNamespace()
	}
	var properties = make(map[string]interface{})
	if err := json.Unmarshal([]byte(req.Properties), &properties); err != nil {
		return nil, err
	}
	if req.Template.Namespace == "" {
		req.Template.Namespace = types.DefaultKubeVelaNS
	}
	configItem, err := i.Factory.ParseConfig(ctx, config.NamespacedName(req.Template), config.Metadata{
		NamespacedName: config.NamespacedName{Name: req.Name, Namespace: ns},
		Properties:     properties,
		Alias:          req.Alias, Description: req.Description,
	})
	if err != nil {
		if errors.Is(err, config.ErrTemplateNotFound) {
			return nil, bcode.ErrTemplateNotFound
		}
		return nil, err
	}
	// Convert secret StringData to Data for Auth
	secret := configItem.Secret
	for key, val := range secret.StringData {
		secret.Data[key] = []byte(val)
	}

	ok, err := i.registryHelper.Auth(secret)
	if !ok {
		klog.Errorf("fail to auth the secret %s:%s", configItem.Secret.Name, err.Error())
		return nil, err
	}
	return convertConfig(project, *configItem), nil
}

func (i *imageImpl) GetRepositoryTags(ctx context.Context, project, secretName, repository string) (*registries.RepositoryTags, error) {
	// empty secret means anonymous fetching
	var secret *corev1.Secret
	if len(secretName) != 0 {
		c, err := i.ConfigService.GetConfig(ctx, project, secretName)
		if err != nil {
			klog.Errorf("Fail to get the secret:%s", err.Error())
			return nil, err
		}
		secret = c.Secret
	}

	tags, err := i.registryHelper.ListRepositoryTags(secret, repository)
	if err != nil {
		klog.Errorf("fail to list repository tags %s:%s", repository, err.Error())
		return nil, err
	}

	return &tags, nil
}

// getAccountFromSecret get the username and password from the secret of `kubernetes.io/dockerconfigjson` type
// refer: https://kubernetes.io/docs/tasks/configure-pod-container/pull-image-private-registry/
func getAccountFromSecret(secret corev1.Secret, registryDomain string) (insecure, useHTTP bool, username, password string) {
	if secret.Data != nil {
		// If users use the self-signed certificate, enable the insecure-skip-verify
		insecure = string(secret.Data["insecure-skip-verify"]) == True
		useHTTP = string(secret.Data["protocol-use-http"]) == True
		conf := secret.Data[".dockerconfigjson"]
		if len(conf) > 0 {
			var authConfig map[string]map[string]map[string]string
			if err := json.Unmarshal(conf, &authConfig); err != nil {
				klog.Warningf("fail to unmarshal the secret %s , %s", secret.Name, err.Error())
				return
			}
			if authConfig != nil && authConfig["auths"] != nil && authConfig["auths"][registryDomain] != nil {
				data := authConfig["auths"][registryDomain]
				username = data["username"]
				password = data["password"]
			}
		}
	}
	return
}

func getImageInfo(imageName string, insecure, useHTTP bool, username, password string, info *v1.ImageInfo) error {
	var options []remote.Option
	if username != "" || password != "" {
		basic := &authn.Basic{
			Username: username,
			Password: password,
		}
		options = append(options, remote.WithAuth(basic))
	}
	if insecure {
		options = append(options, remote.WithTransport(&http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				// By default we wrap the transport in retries, so reduce the
				// default dial timeout to 5s to avoid 5x 30s of connection
				// timeouts when doing the "ping" on certain http registries.
				Timeout:   5 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          100,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
			// #nosec G402
			TLSClientConfig: &tls.Config{InsecureSkipVerify: insecure},
		}))
	}

	var parseOptions []name.Option
	if useHTTP {
		parseOptions = append(parseOptions, name.Insecure)
	}
	var err error
	ref, err := name.ParseReference(imageName, parseOptions...)
	if err != nil {
		return err
	}
	image, err := remote.Image(ref, options...)
	if err != nil {
		if strings.Contains(err.Error(), "incorrect username or password") {
			return fmt.Errorf("incorrect username or password")
		}
		var terr *transport.Error
		if errors.As(err, &terr) {
			fmt.Println(terr)
		}
		return err
	}
	info.Manifest, err = image.Manifest()
	if err != nil {
		return fmt.Errorf("fail to get the manifest:%w", err)
	}
	info.Info, err = image.ConfigFile()
	if err != nil {
		return fmt.Errorf("fail to get the config:%w", err)
	}
	for _, l := range info.Manifest.Layers {
		info.Size += l.Size
	}
	info.Size += info.Manifest.Config.Size
	return nil
}
